package discovery

import (
	"github.com/newrelic/bosun/service"
)

// A Discoverer is responsible for findind services that we care
// about. It must have a method to return the list of services, and
// a Run() method that will be invoked when the discovery mechanism(s)
// is/are started.
type Discoverer interface {
	// Returns a slice of services that we discovered
	Services() []service.Service
	// A non-blocking method that runs a discovery loop.
	// The controlling process kicks it off to start discovery.
	Run(chan bool)
}

// A MultiDiscovery is a wrapper around zero or more Discoverers.
// It allows the use of potentially multiple Discoverers in place of one.
type MultiDiscovery struct {
	Discoverers []Discoverer
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

// Kicks off the Run() method for all the discoverers.
func (d *MultiDiscovery) Run(quit chan bool) {
	var quitChans []chan bool

	for _, disco := range d.Discoverers {
		q := make(chan bool)
		quitChans = append(quitChans, q)
		disco.Run(q)
	}

	go func() {
		<-quit
		for _, q := range quitChans {
			// Copy q so we don't change it out from under the goroutine
			go func(q chan bool) {
				q <- true
			}(q)
		}
	}()
}
