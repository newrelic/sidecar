package healthy

import (
	"log"

	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/service"
)

func (m *Monitor) Services(state *catalog.ServicesState) []service.Service {
	var svcList []service.Service

	m.RLock()
	defer m.RUnlock()

	for _, check := range m.Checks {
		if check.Status == HEALTHY && check.ID != "" {
			svc := *state.GetLocalService(check.ID)
			if svc.ID != "" {
				svcList = append(svcList, svc)
			}
		}
	}

	return svcList
}

func (m *Monitor) CheckForService(name string) Check {
	if _, ok := m.ServiceChecks[name]; !ok {
		return Check{}
	}

	return *m.ServiceChecks[name]
}

func (m *Monitor) Watch(svcFun func() []service.Service, nameFun func(*service.Service) string, looper director.Looper) {

	looper.Loop(func() error {
		services := svcFun()

		// Add checks when new services are found
		for _, svc := range services {
			if nameFun(&svc) == "" {
				log.Printf("Cannot extract name for service: %s", svc.Name)
				continue
			}

			if m.Checks[svc.ID] == nil {
				check := m.CheckForService(nameFun(&svc))
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
		m.Unlock()

		return nil
	})
}
