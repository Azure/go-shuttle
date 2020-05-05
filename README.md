# azure-pub-sub
This golang library serves as a wrapper around the [azure-service-bus-go SDK](https://github.com/Azure/azure-service-bus-go) to facilitate the implementation of a pub-sub system on Azure using service bus.

## Listener Examples
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
    ctx,
    handle,
    topicName,
    pubsub.SetSubscriptionName("subName"),
)
```

### Subscribe to a topic with a filter
```
sqlFilter := fmt.Sprintf("destinationId LIKE '%s'", "test")
err := listener.Listen(
    ctx,
    handle,
    topicName,
    pubsub.SetSubscriptionFilter(
        "testFilter",
        servicebus.SQLFilter{Expression: sqlFilter},
    ),
)
```

## Publisher Examples
### Initializing a publisher with a Service Bus connection string
```
topicName := "topic"
publisher, _ := pubsub.NewPublisher(
    topicName,
    pubsub.PublisherWithConnectionString(serviceBusConnectionString),
)
```

### Initializing a publisher with a header
```
topicName := "topic"
publisher, _ := pubsub.NewPublisher(
    topicName,
    pubsub.PublisherWithConnectionString(serviceBusConnectionString),
    // msgs will have a header with the name "headerName" and value from the msg body field "Id"
    pubsub.SetDefaultHeader("headerName", "Id"),
)
```

### Publishing a message
```
cmd := &ClusterStatusChanged{
    Event: Event{Id: uuidVal},
    Command: Command{
        Id:            uuid.New(),
        DestinationId: "destination",
    },
}
// by default the msg header will have "type" == "ClusterStatusChanged"
err := publisher.Publish(ctx, cmd)
```

### Publishing a message with a delay
```
cmd := &ClusterStatusChanged{
    Event: Event{Id: uuidVal},
    Command: Command{
        Id:            uuid.New(),
        DestinationId: "destination",
    },
}
err := publisher.Publish(
    ctx,
    cmd,
    pubsub.SetMessageDelay(5*time.Second),
)
```
