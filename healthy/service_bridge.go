package healthy

import (
	"time"

	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/services_state"
)

const (
	SERVICE_DISCO_INTERVAL = 100 * time.Millisecond
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

func (m *Monitor) Watch(fn func() []service.Service, count int) {
	interval := time.Tick(SERVICE_DISCO_INTERVAL)
	i := 0
	for range interval {
		services := fn()

		// Add checks when new services are found
		for _, svc := range services {
			m.Lock()
			if m.Checks[svc.ID] == nil {
				// Add a check
			}
			m.Unlock()
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
