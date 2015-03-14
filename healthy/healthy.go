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
	Checks map[string]*Check
	sync.RWMutex
}

// A Check defines some information about how to talk to the
// service to determine health. Each Check has a Command that
// is used to actually do the work. The command is invoked each
// interval and passed the arguments stored in the Check. The
// default Check type is an HttpGetCheck and the Args must be
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

func NewCheck(id string) *Check {
	check := Check{
		ID: id,
		Count: 0,
		Type: "http",
		Command: &HttpGetCmd{},
		MaxCount: 1,
	}
	return &check
}

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

func NewMonitor() *Monitor {
	monitor := Monitor{
		CheckInterval: 3 * time.Second,
		Checks: make(map[string]*Check, 5),
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

	m.Checks[check.ID] = check
}

func (m *Monitor) RemoveCheck(name string) {
	m.Lock()
	defer m.Unlock()
	delete(m.Checks, name)
}

// Run the monitoring loop. Takes an argument of how many
// times to run. -1 means to run forever.
func (m *Monitor) Run(count int) {
	interval := time.Tick(m.CheckInterval)
	i := 0
	for range interval {
		log.Printf("Running checks")

		var wg sync.WaitGroup

		wg.Add(len(m.Checks))
		for _, check := range m.Checks {
			// Run all checks in parallel in goroutines
			resultChan := make(chan checkResult, 1)
			go func() {
				result, err := check.Command.Run(check.Args)
				resultChan <-checkResult{result, err}
			}()

			go func(check *Check) {
				// We make the call but we time out if it gets too close to the
				// Monitor's CheckInterval.
				select {
				case result := <-resultChan:
					check.UpdateStatus(result.status, result.err)
				case <-time.After(m.CheckInterval - 1 * time.Millisecond):
					log.Printf("Error, check %s timed out!", check.ID)
					check.UpdateStatus(UNKNOWN, errors.New("Timed out!"))
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
		if count != -1 {
			i = i + 1
			if i >= count {
				return
			}
		}
	}
}

type checkResult struct {
	status int
	err error
}
