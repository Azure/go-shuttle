package shuttle_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2"
)

func ExampleNewManagedSettlingHandler() {
	tokenCredential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		panic(err)
	}
	client, err := azservicebus.NewClient("myservicebus.servicebus.windows.net", tokenCredential, nil)
	if err != nil {
		panic(err)
	}
	receiver, err := client.NewReceiverForSubscription("topic-a", "sub-a", nil)
	if err != nil {
		panic(err)
	}
	lockRenewalInterval := 10 * time.Second
	p := shuttle.NewProcessor(receiver,
		shuttle.NewPanicHandler(nil,
			shuttle.NewRenewLockHandler(receiver, &lockRenewalInterval,
				shuttle.NewManagedSettlingHandler(&shuttle.ManagedSettlingOptions{
					RetryDecision:      &shuttle.MaxAttemptsRetryDecision{MaxAttempts: 2},
					RetryDelayStrategy: &shuttle.ConstantDelayStrategy{Delay: 2 * time.Second},
				}, myManagedSettlementHandler()))), &shuttle.ProcessorOptions{MaxConcurrency: 10})

	ctx, cancel := context.WithCancel(context.Background())
	err = p.Start(ctx)
	if err != nil {
		panic(err)
	}
	cancel()
}

func myManagedSettlementHandler() shuttle.ManagedSettlingFunc {
	count := 0
	return func(ctx context.Context, message *azservicebus.ReceivedMessage) error {
		count++
		if count == 0 {
			// this will abandon the message
			return fmt.Errorf("this will abandon the message, and eventually move it to DLQ")
		}
		return nil // this will complete the message
	}
}
