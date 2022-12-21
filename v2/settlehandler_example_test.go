package v2_test

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	shuttle "github.com/Azure/go-shuttle/v2"
)

func ExampleSettlingHandler() {
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
		shuttle.NewPanicHandler(
			shuttle.NewRenewLockHandler(receiver, &lockRenewalInterval,
				shuttle.NewSettlementHandler(nil, mySettlingHandler()))), &shuttle.ProcessorOptions{MaxConcurrency: 10})

	ctx, cancel := context.WithCancel(context.Background())
	err = p.Start(ctx)
	if err != nil {
		panic(err)
	}
	cancel()
}

func mySettlingHandler() shuttle.Settler {
	return func(ctx context.Context, message *azservicebus.ReceivedMessage) shuttle.Settlement {
		return &shuttle.Complete{}
	}
}
