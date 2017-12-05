package sidecarhttp

import (
	"fmt"
	_ "net/http/pprof"
	"time"

	"github.com/Nitro/sidecar/catalog"
)

// A ServicesState.Listener that we use for the /watch endpoint
type HttpListener struct {
	eventChan chan catalog.ChangeEvent
	name      string
}

func NewHttpListener() *HttpListener {
	return &HttpListener{
		// This should be fine enough granularity for practical purposes
		name: fmt.Sprintf("httpListener-%d", time.Now().UTC().UnixNano()),
		// Listeners must have buffered channels. We'll use a
		// somewhat larger buffer here because of the slow link
		// problem with http
		eventChan: make(chan catalog.ChangeEvent, 50),
	}
}

func (h *HttpListener) Chan() chan catalog.ChangeEvent {
	return h.eventChan
}

func (h *HttpListener) Name() string {
	return h.name
}

func (h *HttpListener) Managed() bool {
	return false
}
