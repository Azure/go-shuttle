package pubsub

import (
	"github.com/keikumata/azure-pub-sub/listener"
)

// NewListener creates a new service bus listener
// deprecated: use listener.New(opts ...listener.ManagementOption)
func NewListener(opts ...listener.ManagementOption) (*listener.Listener, error) {
	return listener.New(opts...)
}
