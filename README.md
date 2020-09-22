# azure-pub-sub
This golang library serves as a wrapper around the [azure-service-bus-go SDK](https://github.com/Azure/azure-service-bus-go) to facilitate the implementation of a pub-sub system on Azure using service bus.

## Conventions & Assumptions
Currently we are assuming that both the publisher and the listener will both use this azure-pub-sub library.
This is because the listener assumes that the struct type of the body is in the header of the message it receives.
This addition is done automatically when using the publisher of this library via reflection.
This is done so that the library user can easily filter out certain event types.
Specifically this is what the message should look like:

```
{
  data: <some data>,
  userProperties: {
     type: <name of the struct type> // used for subscription filters
  }
}
```
This is enforced by the fact that the listener handler's function signature expects the messageType to be there:
```
type Handle func(ctx context.Context, message, messageType string) error
```
If the `type` field from `userProperties` is missing, the listener handler will automatically throw an error saying it is not supported.

In the future we will support raw listener handlers that don't have this restriction to allow for more publisher flexibility.

## Listener Examples
### Initializing a listener with a Service Bus connection string
```
listener, _ := pubsub.NewListener(pubsub.ListenerWithConnectionString(serviceBusConnectionString))
```

### Initializing a listener using Managed Identity
To configure using managed identity with Service Bus, refer to this [link](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity).
Note if you see errors for certain operation, you may have an RBAC issue. Refer to the built-in RBAC roles [here](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity#built-in-rbac-roles-for-azure-service-bus).
#### Using user assigned managed identity
```
listener, _ := pubsub.NewListener(pubsub.ListenerWithManagedIdentity(serviceBusNamespaceName, managedIdentityClientID))
```

#### Using system assigned managed identity
```
// keep managed identity client ID empty to default using system assigned managed identity
listener, _ := pubsub.NewListener(pubsub.ListenerWithManagedIdentity(serviceBusNamespaceName, ""))
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

### Initializing a publisher using Managed Identity
To configure using managed identity with Service Bus, refer to this [link](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity).
Note if you see errors for certain operation, you may have an RBAC issue. Refer to the built-in RBAC roles [here](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity#built-in-rbac-roles-for-azure-service-bus).
#### Using user assigned managed identity
```
topicName := "topic"
publisher, _ := pubsub.NewPublisher(
    topicName,
    pubsub.PublisherWithManagedIdentity(serviceBusNamespaceName, managedIdentityClientID),
)
```

#### Using system assigned managed identity
```
// keep managed identity client ID empty to default using system assigned managed identity
topicName := "topic"
publisher, _ := pubsub.NewPublisher(
    topicName,
    pubsub.PublisherWithManagedIdentity(serviceBusNamespaceName, ""),
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

### Initializing a publisher with duplication detection
Duplication detection cannot be enabled on Service Bus topics that already exist.
Please think about what capabilities you would like on the Service Bus topic up front at creation time.

Note that you need to use this [feature](https://docs.microsoft.com/en-us/azure/service-bus-messaging/duplicate-detection) in conjunction with setting a messageID on each message you send.
Refer to the [Publishing a message with a message ID section](#publishing-a-message-with-a-message-id) on how to do this.

```
topicName := "topic"
dupeDetectionWindow := 5 * time.Minute
publisher, _ := pubsub.NewPublisher(
    topicName,
    pubsub.PublisherWithConnectionString(serviceBusConnectionString),
    pubsub.SetDuplicateDetection(&dupeDetectionWindow), // if window is null then a default of 30 seconds is used
)
```

### Publishing a message
```
cmd := &SomethingHappenedEvent{
    Id: uuid.New(),
    SomeStringField: "value",
}
// by default the msg header will have "type" == "SomethingHappenedEvent"
err := publisher.Publish(ctx, cmd)
```

### Publishing a message with a delay
```
cmd := &SomethingHappenedEvent{
    Id: uuid.New(),
    SomeStringField: "value",
}
err := publisher.Publish(
    ctx,
    cmd,
    pubsub.SetMessageDelay(5*time.Second),
)
```

### Publishing a message with a message ID
The duplication detection feature requires messages to have a messageID, as messageID is the key ServiceBus will de-dupe on.
Refer to the [Initializing a publisher with duplication detection section](#initializing-a-publisher-with-duplication-detection).
```
cmd := &SomethingHappenedEvent{
    Id: uuid.New(),
    SomeStringField: "value",
}
messageID := "someMessageIDWithBusinessContext"
err := publisher.Publish(
    ctx,
    cmd,
    pubsub.SetMessageID(messageID),
)
```
## Dev environment and integration tests

1. copy the `.env.template` to a `.env` at the root of the repository
2. fill in the environment variable in the .env file 
3. run `make test-setup`. that will create the necessary azure resources.
4. run `make build-test-image` and `make push-test-image`. that will build and push the test image to the create private registry
5. run `make test-aci`
