# azure-pub-sub
This golang library serves as a wrapper around the [azure-service-bus-go SDK](https://github.com/Azure/azure-service-bus-go) to facilitate the implementation of a pub-sub system on Azure using service bus.

## Conventions & Assumptions
Currently we are assuming that both the publisher and the listener will both use this azure-pub-sub library.
This is because the listener assumes that the struct type of the body is in the header of the message it receives.
This addition is done automatically when using the publisher of this library via reflection.
This is done so that the library user can easily filter out certain event types.
Specifically this is what the message should look like:

```json
{
  "data": "<some data>",
  "userProperties": {
     "type": "<name of the struct type>" // used for subscription filters
  }
}
```

This is enforced by the fact that the listener handler's function signature expects the messageType to be there:

```golang
type Handle func(ctx context.Context, msg *message.Message) message.Handler
```

If the `type` field from `userProperties` is missing, the listener handler will automatically throw an error saying it is not supported.

In the future we will support raw listener handlers that don't have this restriction to allow for more publisher flexibility.

## Listener Examples

To start receiving messages, you need to create a Listener, and start listening. 
Creating the Listener creates the connections and initialized the token provider.
You start receiving messages when you call Listen(...) and pass a message handler.

### Initializing a listener with a Service Bus connection string
```golang
l, err := listener.New(listener.WithConnectionString(serviceBusConnectionString))
```

### Initializing a listener using Managed Identity
To configure using managed identity with Service Bus, refer to this [link](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity).
Note if you see errors for certain operation, you may have an RBAC issue. Refer to the built-in RBAC roles [here](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity#built-in-rbac-roles-for-azure-service-bus).
#### Using user assigned managed identity
```
l, _ := listener.New(listener.WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
```
Or using the resource id:
```
l, _ := listener.New(listener.WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
```

#### Using system assigned managed identity
Keep the clientID parameter empty
```golang
l, _ := listener.New(listener.WithManagedIdentityClientID(serviceBusNamespaceName, ""))
defer listener.Close(context.Background(()) // stop receiving messages
```

### Start listening : Subscribe to a topic
#### The Handler
The `Handler` is a func that takes in a context and the message, and returns another `Handler` type, represents the result of the handling.

```golang
handler := message.HandlerFunc(func(ctx context.Context, msg *message.Message) message.Handler {
    err := DoSomething(ctx, msg)
    if err != nil {
        return msg.Error(err) //trace the error, and abandon the message. message will be retried
    }
    return msg.Complete() // handling successful. remove message from topic
})

// listen blocks and handle messages from the topic
err := l.Listen(ctx, handler, topicName)

// Note that err occurs when calling l.Close(), because it closes the context, to shut down the listener.
// This is expected as it is the only way to get out of the blocking Lister call.

```
##### Postponed handling of message
In some cases, your message handler can detect that it is not ready to process the message, and needs to retry later: 
```golang
handler := message.HandlerFunc(func(ctx context.Context, msg *message.Message) message.Handler {
    return msg.RetryLater(10*time.Minute)
})

// listen blocks and handle messages from the topic
err := l.Listen(ctx, handler, topicName)
```

Note that this happens in-memory in your service. The receiver is keeping your message and pushing it back to your handler after the given time.
This will not increase the retry count of the message, as the message is not dequeued another time.

#### Start Listening
```golang
err := l.Listen(ctx, handler, topicName)
```

### Subscribe to a topic with a client-supplied name
```golang
err := l.Listen(
    ctx,
    handler,
    topicName,
    listener.SetSubscriptionName("subName"),
)
```

### Subscribe to a topic with a filter
```golang
sqlFilter := fmt.Sprintf("destinationId LIKE '%s'", "test")
err := l.Listen(
    ctx,
    handle,
    topicName,
    listener.SetSubscriptionFilter(
        "testFilter",
        servicebus.SQLFilter{Expression: sqlFilter},
    ),
)
```

### Listen sample with error check and Close()

```
l, err := listener.New(listener.WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
if err != nil {
    return err
}
...
if err := l.Listen(ctx, handler, topicName); err != nil {
    return err
}
defer func() {
    err := l.Close(ctx)
    if err != nil {
        log.Errorf("failed to close listener: %s", err)
    }
}
```

## Publisher Examples
### Initializing a publisher with a Service Bus connection string
```
topicName := "topic"
pub, _ := publisher.New(
    topicName,
    publisher.PublisherWithConnectionString(serviceBusConnectionString),
)
```

### Initializing a publisher using Managed Identity
To configure using managed identity with Service Bus, refer to this [link](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity).
Note if you see errors for certain operation, you may have an RBAC issue. Refer to the built-in RBAC roles [here](https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-managed-service-identity#built-in-rbac-roles-for-azure-service-bus).
#### Using user assigned managed identity
Using Identity ClientID
```
topicName := "topic"
pub, _ := publisher.New(
    topicName,
    publisher.WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID),
)
```
Using Identity ResourceID
```
topicName := "topic"
pub, _ := publisher.New(
    topicName,
    publisher.WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID),
)
```

#### Using system assigned managed identity
Keep the clientID parameter empty
```
topicName := "topic"
pub, _ := publisher.NewPublisher(
    topicName,
    publisher.WithManagedIdentityClientID(serviceBusNamespaceName, ""),
)
```

### Initializing a publisher with a header
```
topicName := "topic"
pub, _ := publisher.New(
    topicName,
    publisher.WithConnectionString(serviceBusConnectionString),
    // msgs will have a header with the name "headerName" and value from the msg body field "Id"
    publisher.SetDefaultHeader("headerName", "Id"),
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
pub, _ := publisher.New(
    topicName,
    publisher.WithConnectionString(serviceBusConnectionString),
    publisher.SetDuplicateDetection(&dupeDetectionWindow), // if window is null then a default of 30 seconds is used
)
```

### Publishing a message
```
cmd := &SomethingHappenedEvent{
    Id: uuid.New(),
    SomeStringField: "value",
}
// by default the msg header will have "type" == "SomethingHappenedEvent"
err := pub.Publish(ctx, cmd)
```

### Publishing a message with a delay
```
cmd := &SomethingHappenedEvent{
    Id: uuid.New(),
    SomeStringField: "value",
}
err := pub.Publish(
    ctx,
    cmd,
    publisher.SetMessageDelay(5*time.Second),
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
err := pub.Publish(
    ctx,
    cmd,
    publisher.SetMessageID(messageID),
)
```
## Dev environment and integration tests

1. copy the `.env.template` to a `.env` at the root of the repository
2. fill in the environment variable in the .env file 
3. run `make test-setup`. that will create the necessary azure resources.
4. run `make integration`. <- build & push image + start integration test run on ACI
