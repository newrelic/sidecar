package envoy

import (
	"github.com/Nitro/sidecar/catalog"
)

const (
	listenerEventBufferSize = 100
)

// Listener is an internal Sidecar listener that will be hooked up when
// config.Envoy.UseGRPCAPI is true. It only needs to know when the state
// changes, ignoring what the actual change was.
type Listener struct {
	eventsChan chan catalog.ChangeEvent
}

// Chan exposes the internal events channel
func (l *Listener) Chan() chan catalog.ChangeEvent {
	return l.eventsChan
}

// Name returns a unique name for this listener
func (l *Listener) Name() string {
	// Be careful not to clash with names assigned automatically in
	// service/service.go -> ListenerName()
	return "internal-envoy"
}

// Managed tells Sidecar that it shouldn't try to automatically remove
// this listener
func (l *Listener) Managed() bool {
	return false
}

// NewListener creates a new Listener instance
func NewListener() *Listener {
	return &Listener{
		// See catalog/url_listener.go -> NewUrlListener() for a similar mechanism.
		// We use a larger buffer here, because, unlike the URL listener, this is
		// all processed internally in the same process, so we can buffer/flush
		// more events faster.
		eventsChan: make(chan catalog.ChangeEvent, listenerEventBufferSize),
	}
}
