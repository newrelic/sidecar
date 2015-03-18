package healthy

import (
	"log"
	"time"

	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/services_state"
)

func (m *Monitor) Services(state *services_state.ServicesState) []service.Service {
	var svcList []service.Service

	m.RLock()
	defer m.RUnlock()

	for _, check := range m.Checks {
		if check.Status == HEALTHY && check.ID != "" {
			svcList = append(svcList, *state.GetLocalService(check.ID))
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

func (m *Monitor) Watch(svcFun func() []service.Service, nameFun func(*service.Service) string,
		count int, interval time.Duration) {
	ticks := time.Tick(interval)
	i := 0
	for range ticks {
		services := svcFun()

		// Add checks when new services are found
		for _, svc := range services {
			if nameFun(&svc) == "" {
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
					log.Printf("%#v\n", check)
					println("Adding")
					m.AddCheck(&check)
				}
			}
		}

		m.Lock()
OUTER:
		for _, check := range m.Checks {
			found := false
			for _, svc := range services {
				if svc.ID == check.ID {
					continue OUTER
				}
			}

			// Remove checks for services that are no longer running
			if !found {
				delete(m.Checks, check.ID)
			}
		}
		m.Unlock()
	}
	if count != FOREVER {
		i = i + 1
		if i >= count {
			return
		}
	}
}
