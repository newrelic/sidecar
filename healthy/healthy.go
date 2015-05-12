// A lightweight health-checking module so we can make
// sure that services are running and healthy before
// we announce them to our peers. Has a standard check
// interval for all checks, not configurable per check.

package healthy

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/service"
	"github.com/newrelic/bosun/catalog"
)

const (
	HEALTHY = 0
	SICKLY  = iota
	FAILED  = iota
	UNKNOWN = iota
)

const (
	FOREVER         = -1
	WATCH_INTERVAL  = 200 * time.Millisecond
	HEALTH_INTERVAL = 3 * time.Second
)

// The Monitor is responsible for managing and running Checks.
// It has a fixed check interval that is used for all checks.
// Access must be synchronized so direct access to struct
// members is possible but requires use of the RWMutex.
type Monitor struct {
	Checks           map[string]*Check
	CheckInterval    time.Duration
	DefaultCheckHost string
	sync.RWMutex
}

// A Check defines some information about how to talk to the
// service to determine health. Each Check has a Command that
// is used to actually do the work. The command is invoked each
// interval and passed the arguments stored in the Check. The
// default Check type is an HttpGetCmd and the Args must be
// the URL to pass to the check.
type Check struct {
	// The ID of this check
	ID string

	// The most recent status of this check
	Status int

	// The number of runs it has been in failed state
	Count int

	// The maximum number before we declare that it failed
	MaxCount int

	// String describing the kind of check
	Type string

	// The arguments to pass to the Checker
	Args string

	// The Checker to run to validate this
	Command Checker

	// The last recorded error on this check
	LastError error
}

type Checker interface {
	Run(args string) (int, error)
}

// NewCheck returns a properly configured default Check
func NewCheck(id string) *Check {
	check := Check{
		ID:       id,
		Count:    0,
		Type:     "http",
		Command:  &HttpGetCmd{},
		MaxCount: 1,
		Status:   UNKNOWN,
	}
	return &check
}

// UpdateStatus take the status integer and error and applies them to the status
// of the current Check.
func (check *Check) UpdateStatus(status int, err error) {
	if err != nil {
		log.Printf("Error executing check, status UNKNOWN")
		check.Status = UNKNOWN
		check.LastError = err
	} else {
		check.Status = status
	}

	if status == HEALTHY {
		check.Count = 0
		return
	}

	check.Count = check.Count + 1

	if check.Count >= check.MaxCount {
		check.Status = FAILED
	}
}

func (check *Check) ServiceStatus() int {
	switch check.Status {
	case HEALTHY:
		return service.ALIVE
	case SICKLY:
		return service.ALIVE
	case UNKNOWN:
		return service.UNKNOWN
	default:
		return service.UNHEALTHY
	}
}

// NewMonitor returns a properly configured default configuration of a Monitor.
func NewMonitor(defaultCheckHost string) *Monitor {
	monitor := Monitor{
		Checks:           make(map[string]*Check, 5),
		CheckInterval:    HEALTH_INTERVAL,
		DefaultCheckHost: defaultCheckHost,
	}
	return &monitor
}

// Returns a list of all checks that are in a status other
// than HEALTHY.
func (m *Monitor) Unhealthy() []*Check {
	var list []*Check
	m.RLock()
	defer m.RUnlock()

	for _, check := range m.Checks {
		if check.Status != HEALTHY {
			list = append(list, check)
		}
	}
	return list
}

// Returns a slice of checks that are in the HEALTHY status.
func (m *Monitor) Healthy() []*Check {
	var list []*Check
	m.RLock()
	defer m.RUnlock()

	for _, check := range m.Checks {
		if check.Status == HEALTHY {
			list = append(list, check)
		}
	}
	return list
}

// Add a Check to the list. Handles synchronization.
func (m *Monitor) AddCheck(check *Check) {
	m.Lock()
	defer m.Unlock()
	log.Printf("Adding health check: %s %s\n", check.ID, check.Args)
	m.Checks[check.ID] = check
}

// Removes a Check from the list. Handles synchronization.
func (m *Monitor) RemoveCheck(name string) {
	m.Lock()
	defer m.Unlock()
	delete(m.Checks, name)
}

// MarkService takes a service and mark its Status appropriately based on the
// current check we have configured.
func (m *Monitor) MarkService(svc *service.Service) {
	// We remove checks when encountering a Tombstone record. This
	// prevents us from storing up checks forever. The discovery
	// mechanism must create tombstones when services go away, so
	// this is the best signal we'll get that a check is no longer
	// needed. Assumes we're only health checking _our own_ services.
	if svc.IsTombstone() {
		if _, ok := m.Checks[svc.ID]; ok {
			m.Lock()
			delete(m.Checks, svc.ID)
			m.Unlock()
		}
	} else {
		m.RLock()
		if _, ok := m.Checks[svc.ID]; ok {
			svc.Status = m.Checks[svc.ID].ServiceStatus()
		} else {
			svc.Status = service.UNKNOWN
		}
		m.RUnlock()
	}
}

// Run runs the main monitoring loop. The looper controls the actual run behavior.
func (m *Monitor) Run(state *catalog.ServicesState, looper director.Looper) {
	looper.Loop(func() error {
		log.Printf("Running checks")

		var wg sync.WaitGroup

		wg.Add(len(m.Checks))
		for _, check := range m.Checks {
			// Run all checks in parallel in goroutines
			resultChan := make(chan checkResult, 1)
			go func(check *Check) {
				result, err := check.Command.Run(check.Args)
				resultChan <- checkResult{result, err}
			}(check) // copy check pointer for the goroutine

			go func(check *Check) {
				// We make the call but we time out if it gets too close to the
				// m.CheckInterval.
				select {
				case result := <-resultChan:
					log.Printf("%s: %#v\n", check.ID, result)
					check.UpdateStatus(result.status, result.err)
				case <-time.After(m.CheckInterval - 1*time.Millisecond):
					log.Printf("Error, check %s timed out!", check.ID)
					check.UpdateStatus(UNKNOWN, errors.New("Timed out!"))
				}
				wg.Done()
			}(check) // copy check pointer for the goroutine
		}

		// Let's make sure we don't continue to spool up
		// huge quantities of goroutines. Wait on all of them
		// to complete before moving on. This could slow down
		// our check loop if something doesn't time out properly.
		wg.Wait()

		// Actually update the status of all the services we know
		// about. This works against only the local services.
		state.EachLocalService(func(hostname *string, id *string, svc *service.Service){
			m.MarkService(svc)
		})

		return nil
	})
}

type checkResult struct {
	status int
	err    error
}
