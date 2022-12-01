package v2

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

func MyHandler() HandlerFunc {
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		// logic
		err := settler.CompleteMessage(ctx, message, nil)
		panic(err) // note: don't panic
	}
}

func ExampleProcessor() {
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
	p := NewProcessor(receiver,
		NewPanicHandler(
			NewRenewLockHandler(receiver, &lockRenewalInterval,
				MyHandler())), &ProcessorOptions{MaxConcurrency: 10})

	ctx, cancel := context.WithCancel(context.Background())
	err = p.Start(ctx)
	if err != nil {
		panic(err)
	}
	cancel()
}
