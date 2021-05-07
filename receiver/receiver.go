package receiver

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/NinesStack/sidecar/catalog"
	"github.com/NinesStack/sidecar/service"
	"github.com/mohae/deepcopy"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

const (
	RELOAD_HOLD_DOWN = 5 * time.Second // Reload at worst every 5 seconds
)

type Receiver struct {
	StateLock      sync.Mutex
	ReloadChan     chan time.Time
	CurrentState   *catalog.ServicesState
	LastSvcChanged *service.Service
	OnUpdate       func(state *catalog.ServicesState)
	Looper         director.Looper
	Subscriptions  []string
}

func NewReceiver(capacity int, onUpdate func(state *catalog.ServicesState)) *Receiver {
	return &Receiver{
		ReloadChan: make(chan time.Time, capacity),
		OnUpdate:   onUpdate,
		Looper:     director.NewImmediateTimedLooper(director.FOREVER, RELOAD_HOLD_DOWN, make(chan error)),
	}
}

// Check all the state transitions and only update HAproxy when a change
// will affect service availability.
func ShouldNotify(oldStatus int, newStatus int) bool {
	log.Debugf("Checking event. OldStatus: %s NewStatus: %s",
		service.StatusString(oldStatus), service.StatusString(newStatus),
	)

	// Compare old and new states to find significant changes only
	switch newStatus {
	case service.ALIVE:
		return true
	case service.TOMBSTONE:
		return true
	case service.UNKNOWN:
		if oldStatus == service.ALIVE {
			return true
		}
	case service.UNHEALTHY:
		if oldStatus == service.ALIVE {
			return true
		}
	case service.DRAINING:
		return true
	default:
		log.Errorf("Got unknown service change status: %d", newStatus)
		return false
	}

	log.Debugf("Skipped HAproxy update due to state machine check")
	return false
}

// Used to fetch the current state from a Sidecar endpoint, usually
// on startup of this process, when the currentState is empty.
func FetchState(url string) (*catalog.ServicesState, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("Bad status code on state fetch: %d", resp.StatusCode)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	state, err := catalog.Decode(bytes)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// IsSubscribed allows a receiver to filter incoming events by service name
func (rcvr *Receiver) IsSubscribed(svcName string) bool {
	// If we didn't specify any specifically, then we want them all
	if len(rcvr.Subscriptions) < 1 {
		return true
	}

	for _, subName := range rcvr.Subscriptions {
		if subName == svcName {
			return true
		}
	}

	return false
}

// Subscribe is not synchronized and should not be called dynamically. This
// is generally used at setup of the Receiver, before any events begin arriving.
func (rcvr *Receiver) Subscribe(svcName string) {
	for _, subName := range rcvr.Subscriptions {
		if subName == svcName {
			return
		}
	}

	rcvr.Subscriptions = append(rcvr.Subscriptions, svcName)
}

// ProcessUpdates loops forever, processing updates to the state.
// By the time we get here, the HTTP UpdateHandler has already set the
// CurrentState to the newest state we know about. Here we'll try to group
// updates together to prevent repeatedly updating on a series of events that
// arrive in a row.
func (rcvr *Receiver) ProcessUpdates() {
	if rcvr.Looper == nil {
		log.Error("Unable to ProcessUpdates(), Looper is nil in receiver!")
		return
	}

	rcvr.Looper.Loop(func() error {
		// Batch up to RELOAD_BUFFER number updates into a
		// single update.
		first := <-rcvr.ReloadChan
		pending := len(rcvr.ReloadChan)

		// Call the callback
		if rcvr.OnUpdate == nil {
			log.Error("OnUpdate() callback not defined!")
		} else {
			rcvr.StateLock.Lock()
			// Copy the state while locked so we don't have it change
			// under us while writing and we don't hold onto the lock the
			// whole time we're writing to disk (e.g. in haproxy-api).
			tmpState := deepcopy.Copy(rcvr.CurrentState).(*catalog.ServicesState)
			rcvr.StateLock.Unlock()

			rcvr.OnUpdate(tmpState)
		}

		// We just flushed the most recent state, dump all the
		// pending items up to that point.
		var reload time.Time
		for i := 0; i < pending; i++ {
			reload = <-rcvr.ReloadChan
		}

		if pending > 0 {
			log.Infof("Skipped %d messages between %s and %s", pending, first, reload)
		}

		// Don't notify more frequently than every RELOAD_HOLD_DOWN period. When a
		// deployment rolls across the cluster it can trigger a bunch of groupable
		// updates. The Looper handles the sleep after the return nil.
		log.Debug("Holding down...")

		return nil
	})
}

// EnqueueUpdate puts a new timestamp on the update channel, to be processed in a
// goroutine that runs the ProcessUpdates function.
func (rcvr *Receiver) EnqueueUpdate() {
	rcvr.ReloadChan <- time.Now().UTC()
}

// FetchInitialState is used at startup to bootstrap initial state from Sidecar.
func (rcvr *Receiver) FetchInitialState(stateUrl string) error {
	rcvr.StateLock.Lock()
	defer rcvr.StateLock.Unlock()

	log.Info("Fetching initial state on startup...")
	state, err := FetchState(stateUrl)
	if err != nil {
		return err
	} else {
		log.Info("Successfully retrieved state")
		rcvr.CurrentState = state
		if rcvr.OnUpdate == nil {
			log.Error("OnUpdate() callback not defined!")
		} else {
			rcvr.OnUpdate(state)
		}
	}

	return nil
}
