package healthy

import (
	"fmt"
	"log"

	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
)

const (
	DEFAULT_STATUS_HOST     = "localhost"
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
		}

		if check.Status == HEALTHY && check.ID != "" {
			svc := state.GetLocalService(check.ID)
			if svc == nil {
				continue
			}
			if svc.ID != "" {
				svcList = append(svcList, *svc)
			}
		} else {
			log.Printf("Unhealthy service: %s\n", check.ID)
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

func (m *Monitor) CheckForService(svc *service.Service) Check {
	port := findFirstTCPPort(svc)
	if port == nil {
		return Check{ID: svc.ID}
	}

	url := fmt.Sprintf("http://%v:%v%v", DEFAULT_STATUS_HOST, port.Port, DEFAULT_STATUS_ENDPOINT)
	return Check{
		ID:      svc.ID,
		Type:    "HttpGet",
		Args:    url,
		Command: &HttpGetCmd{},
	}
}

func (m *Monitor) Watch(svcFun func() []service.Service, looper director.Looper) {
	looper.Loop(func() error {
		services := svcFun()

		// Add checks when new services are found
		for _, svc := range services {
			if m.Checks[svc.ID] == nil {
				check := m.CheckForService(&svc)
				if check.Command == nil {
					log.Printf(
						"Error: Attempted to add %s (id: %s) but no check configured!",
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
		for _, check := range m.Checks {
			for _, svc := range services {
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
