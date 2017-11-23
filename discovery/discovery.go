package discovery

import (
	"time"

	"github.com/Nitro/sidecar/service"
	"github.com/relistan/go-director"
)

const (
	SLEEP_INTERVAL = 1 * time.Second
)

// A ChangeListener is a service that will receive service change events
// over the HTTP interface.
type ChangeListener struct {
	Name string    // Name to be represented in the Listeners list
	Port int64     // Port of the service to send events to
}

// A Discoverer is responsible for finding services that we care
// about. It must have a method to return the list of services, and
// a Run() method that will be invoked when the discovery mechanism(s)
// is/are started. It will also return the correct health check for
// a service and can allow services to subscribe to Sidecar events.
type Discoverer interface {
	// Returns a slice of services that we discovered
	Services() []service.Service
	// Get the health check and health check args for a service
	HealthCheck(svc *service.Service) (string, string)
	// Services which run on the same host and want to receive
	// Sidecar service change events
	Listeners() []ChangeListener
	// A non-blocking method that runs a discovery loop.
	// The controlling process kicks it off to start discovery.
	Run(director.Looper)
}

// A MultiDiscovery is a wrapper around zero or more Discoverers.
// It allows the use of potentially multiple Discoverers in place of one.
type MultiDiscovery struct {
	Discoverers []Discoverer
}

// Get the health check and health check args for a service
func (d *MultiDiscovery) HealthCheck(svc *service.Service) (string, string) {
	for _, disco := range d.Discoverers {
		if healthCheck, healthCheckArgs := disco.HealthCheck(svc); healthCheck != "" {
			return healthCheck, healthCheckArgs
		}

	}
	return "", ""
}

// Aggregates all the service slices from the discoverers
func (d *MultiDiscovery) Services() []service.Service {
	var aggregate []service.Service

	for _, disco := range d.Discoverers {
		services := disco.Services()
		if len(services) > 0 {
			aggregate = append(aggregate, services...)
		}
	}

	return aggregate
}

// Aggreates all the Listeners() output from the discoverers
func (d *MultiDiscovery) Listeners() []ChangeListener {
	var aggregate []ChangeListener

	for _, disco := range d.Discoverers {
		subscribers := disco.Listeners()
		if len(subscribers) > 0 {
			aggregate = append(aggregate, subscribers...)
		}
	}

	return aggregate
}

// Kicks off the Run() method for all the discoverers.
func (d *MultiDiscovery) Run(looper director.Looper) {
	var loopers []director.Looper

	for _, disco := range d.Discoverers {
		l := director.NewTimedLooper(director.FOREVER, SLEEP_INTERVAL, make(chan error))
		loopers = append(loopers, l)
		disco.Run(l)
	}

	looper.Loop(func() error {
		return nil
	})

	for _, l := range loopers {
		l.Quit()
	}
}
