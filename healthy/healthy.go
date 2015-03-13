// A lightweight health-checking module so we can make
// sure that services are running and healthy before
// we announce them to our peers. Has a standard check
// interval for all checks, not configurable per check.

package healthy

import (
	"log"
	"sync"
	"time"
)

const (
	HEALTHY = 0
	SICKLY  = iota
	FAILED  = iota
	UNKNOWN = iota
)

const (
	FOREVER = 0
)

type Monitor struct {
	CheckInterval time.Duration
	Checks []*Check
	sync.RWMutex
}

type Check struct {
	Status int
	Count int
	Type string
	Args string
	Command Checker
}

type Checker interface {
	Run(args string) (int, error)
}

func NewCheck() *Check {
	check := Check{
		Count: 0,
		Type: "http",
		Command: &HttpCheck{},
	}
	return &check
}

func NewMonitor() *Monitor {
	monitor := Monitor{
		CheckInterval: 3 * time.Second,
		Checks: make([]*Check, 0),
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

func (m *Monitor) AddCheck(check *Check) {
	m.Lock()
	defer m.Unlock()

	m.Checks = append(m.Checks, check)
}

// Run the monitoring loop. Takes an argument of how many
// times to run. -1 means to run forever.
func (m *Monitor) Run(count int) {
	c := time.Tick(m.CheckInterval)
	i := 0
	for range c {
		log.Printf("Running checks")

		var wg sync.WaitGroup

		m.Lock()
		defer m.Unlock()

		wg.Add(len(m.Checks))
		for _, check := range m.Checks {
			// Run all checks in parallel in goroutines
			go func(check *Check) {
				// TODO add timeout around this call
				result, err := check.Command.Run(check.Args)
				if err != nil {
					log.Printf("Error executing check, status UNKNOWN")
					check.Status = UNKNOWN
				} else {
					check.Status = result
				}
				wg.Done()
			}(check) // copy check ptr for the goroutine
		}

		// Let's make sure we don't continue to spool up
		// huge quantities of goroutines by waiting on all of them
		// to complete before moving on. This could slow down
		// our check loop if something doesn't time out properly.
		wg.Wait()
		// Don't increment in this case or we'll stop on maxint rollover
		if count != - 1 {
			i = i + 1
			if i >= count {
				return
			}
		}
	}
}

type HttpCheck struct {}

func (h *HttpCheck) Run(args string) (int, error) {
	return HEALTHY, nil
}
