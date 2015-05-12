package healthy

import (
	"fmt"
	"log"

	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
)

const (
	DEFAULT_STATUS_ENDPOINT = "/status/check"
)

func (m *Monitor) Services(state *catalog.ServicesState) []service.Service {
	var svcList []service.Service

	m.RLock()
	defer m.RUnlock()

	for _, check := range m.Checks {
		if check == nil {
			log.Printf("Error: got nil check!")
			continue
		}

		if state == nil {
			log.Printf("Skipping checking for service, catalog is nil")
			continue
		}

		if check.ID == "" {
			continue
		}

		// We return all services that are not FAILED
		if check.Status == HEALTHY || check.Status == SICKLY {
			svc := state.GetLocalService(check.ID)
			if svc == nil {
				continue
			}

			if svc.ID != "" {
				svcList = append(svcList, *svc)
			}
		}
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
		// We remove checks when encountering a Tombstone record. This
		// prevents us from storing up checks forever. The discovery
		// mechanism must create tombstones when services go away, so
		// this is the best signal we'll get that a check is no longer
		// needed. Assumes we're only health checking _our own_ services.
		for _, check := range m.Checks {
			for _, svc := range services {
				// If it's gone, or tombstoned...
				if svc.ID == check.ID && !svc.IsTombstone() {
					continue OUTER
				}
			}

			// Remove checks for services that are no longer running
			delete(m.Checks, check.ID)
		}

		return nil
	})
}
