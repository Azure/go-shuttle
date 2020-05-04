# azure-pub-sub
This golang library serves as a wrapper around the [azure-service-bus-go SDK](https://github.com/Azure/azure-service-bus-go) to facilitate the implementation of a pub-sub system on Azure using service bus.

## Examples
### Initializing a listener with a Service Bus connection string
```
listener, _ := pubsub.NewListener(pubsub.ListenerWithConnectionString(serviceBusConnectionString))
```

### Subscribe to a topic
```
err := listener.Listen(ctx, handle, topicName)
```

### Subscribe to a topic with a client-supplied name
```
err := listener.Listen(
    ctx, handle,
    topicName,
    pubsub.SetSubscriptionName("subName"),
)
```

### Subscribe to a topic with a filter
```
sqlFilter := fmt.Sprintf("destinationId LIKE '%s'", "test")
err := listener.Listen(
    ctx, handle,
    topicName,
    pubsub.SetSubscriptionFilter(
        "testFilter",
        servicebus.SQLFilter{Expression: sqlFilter},
    ),
)
```
