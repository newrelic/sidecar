package healthy

import (
	"fmt"
	"log"

	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/service"
)

const (
	DEFAULT_STATUS_ENDPOINT = "/status/check"
)

func (m *Monitor) Services() []service.Service {
	var svcList []service.Service

	if m.DiscoveryFn == nil {
		log.Printf("Error: DiscoveryFn not defined!")
		return []service.Service{service.Service{}}
	}

	for _, svc := range m.DiscoveryFn() {
		if svc.ID == "" {
			log.Printf("Error: monitor found empty service ID")
			continue
		}

		m.MarkService(&svc)
		svcList = append(svcList, svc)
	}

	return svcList
}

func findFirstTCPPort(svc *service.Service) *service.Port {
	for _, port := range svc.Ports {
		if port.Type == "tcp" {
			return &port
		}
	}
	return nil
}

// CheckForService returns a Check that has been properly configured for this
// particular service. The default is to return an HTTP check on the first
// TCP port on the endpoint set in DEFAULT_STATUS_ENDPOINT.
func (m *Monitor) CheckForService(svc *service.Service) Check {
	port := findFirstTCPPort(svc)
	if port == nil {
		return Check{ID: svc.ID}
	}

	url := fmt.Sprintf("http://%v:%v%v", m.DefaultCheckHost, port.Port, DEFAULT_STATUS_ENDPOINT)
	return Check{
		ID:      svc.ID,
		Type:    "HttpGet",
		Args:    url,
		Command: &HttpGetCmd{},
	}
}

// Watch loops over a list of services and adds checks for services we don't already
// know about. It then removes any checks for services which have gone away. All
// services are expected to be local to this node.
func (m *Monitor) Watch(svcFun func() []service.Service, looper director.Looper) {
	m.DiscoveryFn = svcFun // Store this so we can use it from Services()

	looper.Loop(func() error {
		services := svcFun()

		// Add checks when new services are found
		for _, svc := range services {
			if m.Checks[svc.ID] == nil {
				check := m.CheckForService(&svc)
				if check.Command == nil {
					log.Printf(
						"Attempted to add %s (id: %s) but no check configured!",
						svc.Name, svc.ID,
					)
				} else {
					m.AddCheck(&check)
				}
			}
		}

		m.Lock()
		defer m.Unlock()
	OUTER:
		// We remove checks when encountering a missing service. This
		// prevents us from storing up checks forever. This is the only
		// way we'll find out about a service going away.
		for _, check := range m.Checks {
			for _, svc := range services {
				// Continue if we have a matching service/check pair
				if svc.ID == check.ID {
					continue OUTER
				}
			}

			// Remove checks for services that are no longer running
			delete(m.Checks, check.ID)
		}

		return nil
	})
}
