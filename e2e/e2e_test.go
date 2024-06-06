package e2e

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2"
	"github.com/stretchr/testify/require"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (s *SBSuite) TestPublishAndListen_ConcurrentLockRenewal() {
	t := s.T()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	topicName := s.ApplyPrefix("lock-renewal-topic")
	subscriptionName := "sub"
	s.EnsureTopic(ctx, t, s.sbAdminClient, topicName)
	s.EnsureTopicSubscription(ctx, t, s.sbAdminClient, topicName, subscriptionName)
	success := make(chan bool)
	sendCount := 25
	go func() {
		t.Logf("creating receiver...")
		receiver, err := s.sbClient.NewReceiverForSubscription(topicName, subscriptionName, nil)
		require.NoError(t, err)
		lockRenewalInterval := 2 * time.Second
		p := shuttle.NewProcessor(receiver,
			shuttle.NewPanicHandler(nil,
				shuttle.NewRenewLockHandler(&shuttle.LockRenewalOptions{Interval: &lockRenewalInterval},
					shuttle.NewSettlementHandler(nil,
						testHandler(t, success, sendCount)))), &shuttle.ProcessorOptions{MaxConcurrency: 25})

		t.Logf("start processor...")
		err = p.Start(ctx)
		t.Logf("processor exited: %s", err)
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

func (s *SBSuite) TestSenderFailOverAndMultiProcessor() {
	t := s.T()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	topicName := s.ApplyPrefix("failover-topic")
	subscriptionName := "sub"
	s.EnsureTopic(ctx, t, s.sbAdminClient, topicName)
	s.EnsureTopic(ctx, t, s.sbFailOverAdminClient, topicName)
	s.EnsureTopicSubscription(ctx, t, s.sbAdminClient, topicName, subscriptionName)
	s.EnsureTopicSubscription(ctx, t, s.sbFailOverAdminClient, topicName, subscriptionName)
	success := make(chan bool)
	sendCountPerNamespace := 10

	go func() {
		t.Logf("creating receiver...")
		receiver, err := s.sbClient.NewReceiverForSubscription(topicName, subscriptionName, nil)
		require.NoError(t, err)
		failoverReceiver, err := s.sbFailOverClient.NewReceiverForSubscription(topicName, subscriptionName, nil)
		require.NoError(t, err)
		lockRenewalInterval := 2 * time.Second
		receivers := []*shuttle.ReceiverEx{
			shuttle.NewReceiverEx("primary", receiver),
			shuttle.NewReceiverEx("failover", failoverReceiver),
		}
		p := shuttle.NewMultiProcessor(receivers,
			shuttle.NewPanicHandler(nil,
				shuttle.NewRenewLockHandler(&shuttle.LockRenewalOptions{Interval: &lockRenewalInterval},
					shuttle.NewSettlementHandler(nil,
						testHandler(t, success, sendCountPerNamespace*2)))), &shuttle.ProcessorOptions{MaxConcurrency: 20})

		t.Logf("start processor...")
		err = p.Start(ctx)
		t.Logf("processor exited: %s", err)
	}()

	// Create initial sender and send a batch of messages
	sender, err := s.sbClient.NewSender(topicName, nil)
	require.NoError(t, err)
	shuttleSender := shuttle.NewSender(sender, nil)
	for i := 0; i < sendCountPerNamespace; i++ {
		err = shuttleSender.SendMessage(ctx, []byte("{'value':'some message before failover'}"))
		require.NoError(t, err)
	}

	// Fail over to a new sender and send another batch of messages
	newSender, err := s.sbFailOverClient.NewSender(topicName, nil)
	require.NoError(t, err)
	shuttleSender.FailOver(newSender)
	for i := 0; i < sendCountPerNamespace; i++ {
		err = shuttleSender.SendMessage(ctx, []byte("{'value':'some message after failover'}"))
		require.NoError(t, err)
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
		t.Logf("current send count: %d", count)
		time.Sleep(10 * time.Second)
		if count == uint32(expectedCount) {
			success <- true
		}
		t.Logf("completing message...")
		return &shuttle.Complete{}
	}
}
