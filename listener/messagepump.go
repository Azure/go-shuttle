package listener

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

//in pratice will be a subscription or a queue but implmented by receivingEntity for now
type renewer interface {
	RenewLocks(ctx context.Context, messages ...*servicebus.Message) error
}

// Listener is a struct to contain service bus entities relevant to subscribing to a publisher topic
type MessagePump struct {
	concurrency  int
	autorenew    time.Duration
	lockDuration time.Duration
	renewer      renewer
}

func NewMessagePump() {

}

// Listen waits for a message from the Service Bus Topic subscription
func (mp *MessagePump) Pump(handler servicebus.HandlerFunc) servicebus.HandlerFunc {

	//message pump doesn't set prefetch count. Should we try and check it and make sure

	semaphore := make(chan bool, mp.concurrency)
	//for each messge start two go routines one to handle the message and anotehr to renew lock till we're done
	return servicebus.HandlerFunc(func(ctx context.Context, msg *servicebus.Message) error {
		semaphore <- true                                     //should make sure we block at l.concurrency
		ctx, cancel := context.WithTimeout(ctx, mp.autorenew) //
		go func() {
			defer cancel()
			handler(ctx, msg)
		}()
		//renew locks periodically
		go func() {
			defer func() {
				cancel()
				<-semaphore //putting this here or in above is important
			}()
			lockexpire := time.After(mp.lockDuration)
			for {
				select {
				case <-lockexpire:
					return
				case <-ctx.Done():
					return
				case <-time.After(mp.lockDuration / 5):
					if err := mp.renewer.RenewLocks(ctx, msg); err != nil {
						//log this failure?
						continue
					}
					lockexpire = time.After(mp.lockDuration) //reset
				}
			}
		}()
		return nil
	})
}
