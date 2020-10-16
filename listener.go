package shuttle

import (
	"github.com/Azure/go-shuttle/listener"
)

// NewListener creates a new service bus listener
// deprecated: use listener.New(opts ...listener.ManagementOption)
func NewListener(opts ...listener.ManagementOption) (*listener.Listener, error) {
	return listener.New(opts...)
}
