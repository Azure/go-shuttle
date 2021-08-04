package deadletter

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/prometheus/listener"
	"github.com/devigned/tab"
)

const timeout = time.Second * 2

type Monitor struct {
	topicName        string
	subscriptionName string
	counter          Counter
	recorder         listener.DLQRecorder
}

func (m *Monitor) Run(ctx context.Context) {
	for ctx.Err() == nil {
		callCtx, cancel := context.WithTimeout(context.Background(), timeout)
		m.ReportDeadLetterCount(callCtx) // ignore error as we just log it in internal the span at this point
		cancel()
	}
}

func (m *Monitor) ReportDeadLetterCount(ctx context.Context) error {
	callCtx, span := tab.StartSpan(ctx, "go-shuttle.deadLetterMonitor.Run")
	defer span.End()
	countDetails, err := m.counter.CountDetails(callCtx)
	if err != nil {
		span.Logger().Error(err)
		return err
	}
	dlqCount := countDetails.DeadLetterMessageCount
	if dlqCount != nil {
		m.recorder.SetDeadLetterMessageCount(m.topicName, m.subscriptionName, float64(*dlqCount))
	}
	transferDlqCount := countDetails.TransferDeadLetterMessageCount
	if transferDlqCount != nil {
		m.recorder.SetTransferDeadLetterMessageCount(m.topicName, m.subscriptionName, float64(*transferDlqCount))
	}
	return nil
}

type Counter interface {
	CountDetails(ctx context.Context) (*servicebus.CountDetails, error)
}

func StartMonitoring(ctx context.Context, c Counter, topicName, subscriptionName string) {
	m := &Monitor{
		counter:          c,
		topicName:        topicName,
		subscriptionName: subscriptionName,
	}
	m.Run(ctx)
}
