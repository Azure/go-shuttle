# go-shuttle
go-shuttle serves as a wrapper around the [azure service-bus go SDK](https://github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus) to facilitate the implementation of a pub-sub pattern on Azure using service bus.

> NOTE: This library is in early development and should be considered experimental. The api is still moving and can change. 
> We do have breaking changes in v0.*. Use at your own risks.

## Conventions & Assumptions
We are assuming that both the publisher and the listener are using go-shuttle

## Processor

The processor handles the message pump and feeds your message handler.
It allows concurrent message handling and provides a message handler middleware pipeline to compose message handling behavior

### Concurrent message handling

```golang
// ProcessorOptions configures the processor
// MaxConcurrency defaults to 1. Not setting MaxConcurrency, or setting it to 0 or a negative value will fallback to the default.
// ReceiveInterval defaults to 2 seconds if not set.
type ProcessorOptions struct {
  MaxConcurrency  int
  ReceiveInterval *time.Duration
}
```

see [Processor example](v2/processor_test.go)

## Middlewares:
GoSHuttle provides a few middleware to simplify the implementation of the message handler in the application code

### SettlementHandler

Forces the application handler implementation to return a Settlement. This prevents 2 common mistakes:
- Exit the handler without settling the message.
- Settling the message, but not exiting the handler (forgetting to return after calling abandon, for example)

see [SettlementHandler example](v2/settlehandler_example_test.go)

### ManagedSettlingHandler

Allows your handler implementation to just return error. the ManagedSettlingHandler settles the message based on your 
handler return value.
- nil = CompleteMessage
- error -> Abandon or Deadletter, depending on your configuration of the handler.

see [ManagedSettlingHandler](v2/managedsettling_example_test.go)

### Enable automatic message lock renewal

This middleware will renew the lock on each message every 30 seconds until the message is `Completed` or `Abandoned`.

```golang
renewInterval := 30 * time.Second
shuttle.NewRenewLockHandler(receiver, &lockRenewalInterval, handler)
```

see setup in [Processor example](v2/processor_test.go)

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft 
trademarks or logos is subject to and must follow 
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.
