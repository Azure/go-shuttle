package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-amqp-common-go/v3/auth"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-shuttle/prometheus/listener"
	"github.com/Azure/go-shuttle/tracing"
	"github.com/devigned/tab"
	"golang.org/x/sync/errgroup"
)

type HttpLockRenewer struct {
	entityBasePath string
	tokenProvider  auth.TokenProvider
	sender         autorest.Sender
}

type EntityInfoProvider interface {
	servicebus.EntityManagementAddresser
}

func NewHttpLockRenewer(namespaceName string, eip servicebus.EntityManagementAddresser, tp auth.TokenProvider) *HttpLockRenewer {
	entityPath := strings.TrimSuffix(eip.ManagementPath(), "/$management")
	entityUrl := fmt.Sprintf("https://%s.servicebus.windows.net/%s", namespaceName, entityPath)
	return &HttpLockRenewer{
		entityBasePath: entityUrl,
		tokenProvider:  tp,
		sender: autorest.DecorateSender(autorest.CreateSender(),
			autorest.DoErrorUnlessStatusCode(http.StatusOK),
			autorest.DoCloseIfError(),
			autorest.DoRetryForAttempts(3, time.Millisecond*500),
			traceDecorator),
	}
}

func (h *HttpLockRenewer) getUrl(message *servicebus.Message) string {
	return fmt.Sprintf("%s/messages/%s/%s",
		h.entityBasePath,
		message.ID,
		message.LockToken)
}

func (h *HttpLockRenewer) RenewLocks(ctx context.Context, messages ...*servicebus.Message) error {
	token, err := h.tokenProvider.GetToken(h.entityBasePath)
	if err != nil {
		return fmt.Errorf("failed to acquire token for %s", h.entityBasePath)
	}
	if len(messages) == 1 {
		return h.RenewLock(ctx, token, messages[0])
	}

	g, ctx := errgroup.WithContext(ctx)
	for _, m := range messages {
		g.Go(func() error { return h.RenewLock(ctx, token, m) })
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

var traceDecorator = func(s autorest.Sender) autorest.Sender {
	return autorest.SenderFunc(func(r *http.Request) (*http.Response, error) {
		ctx, span := tab.StartSpan(r.Context(), "go-shuttle.httplockrenewer.RenewLock")
		defer span.End()
		fmt.Println("renewing lock", r.URL.Path)
		resp, err := s.Do(r.WithContext(ctx))
		if err != nil {
			span.Logger().Error(err)
		} else {
			span.AddAttributes(tab.Int64Attribute("status-code", int64(resp.StatusCode)))
		}
		return resp, err
	})
}

func (h *HttpLockRenewer) RenewLock(ctx context.Context, token *auth.Token, message *servicebus.Message) error {
	req, err := h.renewLockRequest(ctx, token, message)
	if err != nil {
		return err
	}
	resp, err := autorest.SendWithSender(h.sender, req)
	if err := autorest.Respond(resp, autorest.ByClosing()); err != nil {
		return err
	}
	return nil
}

func (h *HttpLockRenewer) renewLockRequest(ctx context.Context, token *auth.Token, message *servicebus.Message) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.getUrl(message), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("content-Type", "application/atom+xml;type=entry;charset=utf-8")
	req.Header.Add("Authorization", token.Token)
	return req, err
}

var _ LockRenewer = &HttpLockRenewer{}

var _ servicebus.Handler = (*peekLockRenewer)(nil)

// LockRenewer abstracts the servicebus subscription client where this functionality lives
type LockRenewer interface {
	RenewLocks(ctx context.Context, messages ...*servicebus.Message) error
}

// PeekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type peekLockRenewer struct {
	next                         servicebus.Handler
	lockRenewer                  LockRenewer
	renewalInterval              *time.Duration
	cancelMessageHandlingContext context.CancelFunc
}

func (plr *peekLockRenewer) Handle(ctx context.Context, msg *servicebus.Message) error {
	nextCtx, c := context.WithCancel(ctx)
	plr.cancelMessageHandlingContext = c
	if plr.lockRenewer != nil && plr.renewalInterval != nil {
		go plr.startPeriodicRenewal(ctx, msg)
	}
	return plr.next.Handle(nextCtx, msg)
}

func NewPeekLockRenewer(interval *time.Duration, lockrenewer LockRenewer, next servicebus.Handler) servicebus.Handler {
	if next == nil {
		panic(NextHandlerNilError.Error())
	}
	return &peekLockRenewer{
		next:            next,
		lockRenewer:     lockrenewer,
		renewalInterval: interval,
	}
}

func (plr *peekLockRenewer) startPeriodicRenewal(ctx context.Context, message *servicebus.Message) {
	_, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.peeklock.startPeriodicRenewal", message)
	defer span.End()
	count := 0
	for alive := true; alive; {
		select {
		case <-time.After(*plr.renewalInterval):
			count++
			tab.For(ctx).Debug("Renewing message lock", tab.Int64Attribute("count", int64(count)))
			err := plr.lockRenewer.RenewLocks(ctx, message)
			if err != nil {
				listener.Metrics.IncMessageLockRenewedFailure(message)
				// I don't think this is a problem. the context is canceled when the message processing is over.
				// this can happen if we already entered the interval case when the message is completing.
				fmt.Println(message.LockToken, count, "failed to renew the peek lock", err.Error())
				tab.For(ctx).Info("failed to renew the peek lock", tab.StringAttribute("reason", err.Error()))
				return
			}
			tab.For(ctx).Debug("renewed lock success")
			listener.Metrics.IncMessageLockRenewedSuccess(message)
		case <-ctx.Done():
			tab.For(ctx).Info("Stopping periodic renewal")
			err := ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				listener.Metrics.IncMessageDeadlineReachedCount(message)
			}
			alive = false
		}
	}
}
