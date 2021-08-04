package baseinterfaces

import servicebus "github.com/Azure/azure-service-bus-go"

var _ BasePublisher = &PublisherSettings{}

type BasePublisher interface {
	Namespace() *servicebus.Namespace
	Headers() map[string]string
	SetNamespace(namespace *servicebus.Namespace)
	AppendHeader(k,v string)
}

// PublisherSettings is a struct to contain service bus entities relevant to publishing to a queue
type PublisherSettings struct {
	namespace              *servicebus.Namespace
	headers                map[string]string
}

func (p *PublisherSettings) Namespace() *servicebus.Namespace {
	return p.namespace
}

func (p *PublisherSettings) Headers() map[string]string {
	return p.headers
}

func (p *PublisherSettings) SetNamespace(namespace *servicebus.Namespace) {
	p.namespace = namespace
}

func (p *PublisherSettings) AppendHeader(key, value string) {
	if p.headers == nil {
		p.headers = make(map[string]string)
	}
	p.headers[key] = value
}

