package e2e

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/stretchr/testify/require"

	"github.com/Azure/go-shuttle/v2"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (s *SBSuite) TestPublishAndListen_ConcurrentLockRenewal() {
	t := s.T()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	topicName := s.ApplyPrefix("default-topic")
	subscriptionName := "sub"
	s.EnsureTopic(ctx, t, topicName)
	s.EnsureTopicSubscription(ctx, t, topicName, subscriptionName)
	success := make(chan bool)
	sendCount := 25
	go func() {
		t.Logf("creating receiver...")
		receiver, err := s.sbClient.NewReceiverForSubscription(topicName, subscriptionName, nil)
		require.NoError(t, err)
		lockRenewalInterval := 2 * time.Second
		p := shuttle.NewProcessor(receiver,
			shuttle.NewPanicHandler(nil,
				shuttle.NewLockRenewalHandler(receiver, &shuttle.LockRenewalOptions{Interval: &lockRenewalInterval},
					shuttle.NewSettlementHandler(nil,
						testHandler(t, success, sendCount)))), &shuttle.ProcessorOptions{MaxConcurrency: 25})

		t.Logf("start processor...")
		err = p.Start(ctx)
		t.Logf("processor exited: %s", err)
		require.EqualError(t, err, context.DeadlineExceeded.Error())
	}()

	t.Logf("creating sender...")
	sender, err := s.sbClient.NewSender(topicName, nil)
	require.NoError(t, err)
	t.Logf("sending message...")
	for i := 0; i < sendCount; i++ {
		err = sender.SendMessage(ctx, &azservicebus.Message{
			Body: []byte("{'value':'some message'}"),
		}, nil)
		require.NoError(t, err)
		t.Logf("message %d sent...", i)
	}
	select {
	case ok := <-success:
		require.True(t, ok)
	case <-ctx.Done():
		t.Errorf("did not complete the message in time")
	}
}

func testHandler(t *testing.T, success chan bool, expectedCount int) shuttle.Settler {
	var count uint32
	return func(ctx context.Context, message *azservicebus.ReceivedMessage) shuttle.Settlement {
		t.Logf("Processing message.\n Delivery Count: %d\n", message.DeliveryCount)
		t.Logf("ID: %s - Locked Until: %s\n", message.MessageID, message.LockedUntil)
		t.Logf("sleeping...")
		atomic.AddUint32(&count, 1)
		time.Sleep(12 * time.Second)
		t.Logf("completing message...")
		if count == uint32(expectedCount) {
			success <- true
		}
		return &shuttle.Complete{}
	}
}
