package catalog

//go:generate ffjson $GOFILE

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/NinesStack/memberlist"
	"github.com/NinesStack/sidecar/output"
	"github.com/NinesStack/sidecar/service"
	"github.com/armon/go-metrics"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

// catalog handles all of the eventual-consistency mechanisms for
// service discovery state. The ServicesState struct has a mapping of
// servers to Service lists and manages the lifecycle.

const (
	TOMBSTONE_LIFESPAN         = 3 * time.Hour                  // How long we keep tombstones around
	TOMBSTONE_COUNT            = 10                             // Send tombstones at 1 per second 10 times
	ALIVE_COUNT                = 5                              // Send new services at 1 per second 5 times
	TOMBSTONE_SLEEP_INTERVAL   = 2 * time.Second                // Sleep between local service checks
	TOMBSTONE_RETRANSMIT       = 1 * time.Second                // Time between tombstone retranmission
	ALIVE_LIFESPAN             = 1*time.Minute + 20*time.Second // Down if not heard from in 80 seconds
	DRAINING_LIFESPAN          = 10 * time.Minute               // Down if not heard from in 10 minutes
	ALIVE_SLEEP_INTERVAL       = 1 * time.Second                // Sleep between local service checks
	ALIVE_BROADCAST_INTERVAL   = 1 * time.Minute                // Broadcast Alive messages every minute
	LISTENER_EVENT_BUFFER_SIZE = 20                             // The number of events that can be buffered in the listener eventChannel
)

// A ChangeEvent represents the time and hostname that was modified and signals a major
// state change event. It is passed to listeners over the listeners channel in the
// state object.
type ChangeEvent struct {
	Service        service.Service
	PreviousStatus int
	Time           time.Time
}

// Holds the state about one server in our cluster
type Server struct {
	Name        string
	Services    map[string]*service.Service
	LastUpdated time.Time
	LastChanged time.Time
}

// Returns a pointer to a properly configured Server
func NewServer(name string) *Server {
	server := &Server{
		Name: name,
		// Pre-create for 5 services per host
		Services:    make(map[string]*service.Service, 5),
		LastUpdated: time.Unix(0, 0),
		LastChanged: time.Unix(0, 0),
	}

	return server
}

// Holds the state about all the servers in the cluster
type ServicesState struct {
	Servers             map[string]*Server
	LastChanged         time.Time
	ClusterName         string
	Hostname            string
	Broadcasts          chan [][]byte        `json:"-"`
	ServiceMsgs         chan service.Service `json:"-"`
	listeners           map[string]Listener
	tombstoneRetransmit time.Duration
	sync.RWMutex
}

// A Listener receives update events from state changes.
type Listener interface {
	Chan() chan ChangeEvent // The event channel
	Name() string           // The name of this listener
	Managed() bool          // Is this managed by us? (e.g. auto-added/removed)
}

// Returns a pointer to a properly configured ServicesState
func NewServicesState() *ServicesState {
	var err error
	state := &ServicesState{
		Servers:             make(map[string]*Server, 5),
		Broadcasts:          make(chan [][]byte),
		LastChanged:         time.Unix(0, 0),
		tombstoneRetransmit: TOMBSTONE_RETRANSMIT,
		ServiceMsgs:         make(chan service.Service, 25),
		listeners:           make(map[string]Listener),
	}
	state.Hostname, err = os.Hostname()
	if err != nil {
		log.Errorf("Error getting hostname! %s", err.Error())
	}

	return state
}

// Shortcut for checking if the server has this service or not.
// Note: Not synchronized!
func (server *Server) HasService(id string) bool {
	_, ok := server.Services[id]
	return ok
}

// Return a Marshaled/Encoded byte array that can be deocoded with
// catalog.Decode()
func (state *ServicesState) Encode() []byte {
	jsonData, err := state.MarshalJSON()
	if err != nil {
		log.Error("ERROR: Failed to Marshal state")
		return []byte{}
	}

	return jsonData
}

// ProcessNewServiceMsgs is to be run in a goroutine, and processes incoming
// service notices.
func (state *ServicesState) ProcessServiceMsgs(looper director.Looper) {
	looper.Loop(func() error {
		service := <-state.ServiceMsgs
		state.AddServiceEntry(service)
		return nil
	})
}

// UpdateService enqueues a state update for a given service
func (state *ServicesState) UpdateService(svc service.Service) {
	state.ServiceMsgs <- svc
}

// Shortcut for checking if the Servers map has an entry for this
// hostname.
func (state *ServicesState) HasServer(hostname string) bool {
	_, ok := state.Servers[hostname]
	return ok
}

// A server has left the cluster, so tombstone all of its records
func (state *ServicesState) ExpireServer(hostname string) {
	state.Lock()
	defer state.Unlock()

	if !state.HasServer(hostname) || len(state.Servers[hostname].Services) == 0 {
		log.Infof("No records to expire for %s", hostname)
		return
	}

	hasLiveServices := false
	for _, svc := range state.Servers[hostname].Services {
		if !svc.IsTombstone() {
			hasLiveServices = true
			break
		}
	}

	if !hasLiveServices {
		log.Infof("No records to expire for %s (no live services)", hostname)
		return
	}

	log.Infof("Expiring %s", hostname)

	var tombstones []service.Service

	for _, svc := range state.Servers[hostname].Services {
		previousStatus := svc.Status
		svc.Tombstone()
		state.ServiceChanged(svc, previousStatus, svc.Updated)
		tombstones = append(tombstones, *svc)
	}

	if len(tombstones) < 1 {
		log.Warn("Tried to announce a zero length list of tombstones")
		return
	}

	state.SendServices(
		tombstones,
		director.NewTimedLooper(TOMBSTONE_COUNT, state.tombstoneRetransmit, nil),
	)
}

// Tell the state that a particular service transitioned from one state to another.
func (state *ServicesState) ServiceChanged(svc *service.Service, previousStatus int, updated time.Time) {
	state.serverChanged(svc.Hostname, updated)
	state.NotifyListeners(svc, previousStatus, state.LastChanged)
}

// Tell the state that something changed on a particular server so that it
// can keep the timestamps up to date.
// Note: not synchronized!
func (state *ServicesState) serverChanged(hostname string, updated time.Time) {
	if !state.HasServer(hostname) {
		log.Errorf("Attempt to change a server we don't have! (%s)", hostname)
		return
	}

	state.Servers[hostname].LastUpdated = updated
	state.Servers[hostname].LastChanged = updated
	state.LastChanged = updated
}

// Tell all of our listeners that something changed for a host at
// set timestamp. See AddListener() for information about how channels
// must be configured.
func (state *ServicesState) NotifyListeners(svc *service.Service, previousStatus int, changedTime time.Time) {
	listeners := state.listeners

	if len(listeners) < 1 {
		log.Debugf("Skipping listeners, there are none")
		return
	}

	log.Debugf("Notifying listeners of change at %s", changedTime.String())

	event := ChangeEvent{Service: *svc, PreviousStatus: previousStatus, Time: changedTime}
	for _, listener := range listeners {
		if listener == nil {
			continue
		}

		select {
		case listener.Chan() <- event:
			continue
		default:
			log.Warnf("Can't notify listener (%s). May not be ready yet.", listener.Name())
		}
	}
}

// Add an event listener channel to the list that will be notified on
// major state change events. Channels must be buffered by at least 1
// or they will block. Channels must be ready to receive input.
func (state *ServicesState) AddListener(listener Listener) {
	if listener.Chan() == nil {
		log.Errorf("Refusing to add listener %s with nil channel!", listener.Name())
		return
	}

	if cap(listener.Chan()) < 1 {
		log.Errorf("Refusing to add blocking channel as listener: %s", listener.Name())
		return
	}
	state.Lock()
	defer state.Unlock()

	state.listeners[listener.Name()] = listener
	log.Debugf("AddListener(): added %s, new count %d", listener.Name(), len(state.listeners))
}

// Remove an event listener channel by name. This will find the first
// listener in the list with the specified name and will remove it.
func (state *ServicesState) RemoveListener(name string) error {
	state.Lock()
	defer state.Unlock()

	if _, ok := state.listeners[name]; !ok {
		return fmt.Errorf("no listener found with the name %q", name)
	}

	delete(state.listeners, name)
	log.Debugf("RemoveListener(): removed %s, new count %d", name, len(state.listeners))

	return nil
}

// GetListeners returns a slice containing all the current listeners
func (state *ServicesState) GetListeners() []Listener {
	state.RLock()
	var listeners []Listener
	for _, listener := range state.listeners {
		listeners = append(listeners, listener)
	}
	state.RUnlock()

	return listeners
}

// Take a service and merge it into our state. Correctly handle
// timestamps so we only add things newer than what we already
// know about. Retransmits updates to cluster peers.
func (state *ServicesState) AddServiceEntry(newSvc service.Service) {
	defer metrics.MeasureSince([]string{"services_state", "AddServiceEntry"}, time.Now())

	state.Lock()
	defer state.Unlock()

	if !state.HasServer(newSvc.Hostname) {
		state.Servers[newSvc.Hostname] = NewServer(newSvc.Hostname)
	}

	server := state.Servers[newSvc.Hostname]

	// Only apply changes that are newer or services are missing
	if !server.HasService(newSvc.ID) {
		server.Services[newSvc.ID] = &newSvc
		state.ServiceChanged(&newSvc, service.UNKNOWN, newSvc.Updated)
		state.retransmit(newSvc)
	} else if newSvc.Invalidates(server.Services[newSvc.ID]) {
		// We have to set these even if the status did not change
		server.LastUpdated = newSvc.Updated

		// Store the previous newSvc so we can compare it
		oldEntry := server.Services[newSvc.ID]

		// Make sure we preserve the DRAINING status for services
		if oldEntry.Status == service.DRAINING && newSvc.Status == service.ALIVE {
			newSvc.Status = oldEntry.Status
		}

		// Update the new one
		server.Services[newSvc.ID] = &newSvc

		// When the status changes, the SeviceChanged() method will
		// update all the accounting fields in the state and Server newSvc.
		if oldEntry.Status != newSvc.Status {
			state.ServiceChanged(&newSvc, oldEntry.Status, newSvc.Updated)
		}

		// We tell our gossip peers about the updated service
		// by sending them the record. We're saved from an endless
		// retransmit loop by the Invalidates() call above.
		state.retransmit(newSvc)
	}
}

// GetLocalServiceByID returns a service for a given ID if it
// happens to exist on the current host. Returns an error otherwise.
func (state *ServicesState) GetLocalServiceByID(id string) (service.Service, error) {
	state.RLock()
	defer state.RUnlock()

	if server, ok := state.Servers[state.Hostname]; ok {
		if svc, ok := server.Services[id]; ok {
			return *svc, nil
		}
	}

	return service.Service{},
		fmt.Errorf("service with ID %q not found on host %q", id, state.Hostname)
}

// Merge a complete state struct into this one. Usually used on
// node startup and during anti-entropy operations.
func (state *ServicesState) Merge(otherState *ServicesState) {
	for _, server := range otherState.Servers {
		for _, svc := range server.Services {
			state.UpdateService(*svc)
		}
	}
}

// Take a service we already handled, and drop it back into the
// channel. Backgrounded to not block the caller.
func (state *ServicesState) retransmit(svc service.Service) {
	// We don't retransmit our own events! We're already
	// transmitting them.
	if svc.Hostname == state.Hostname {
		return
	}

	go func() {
		encoded, err := svc.Encode()
		if err != nil {
			log.Errorf("ERROR encoding message to forward: (%s)", err.Error())
			return
		}
		state.Broadcasts <- [][]byte{encoded}
	}()
}

// Pretty-print(ish) a services state struct so a human can read
// it on the terminal. Makes for awesome web apps.
func (state *ServicesState) Format(list *memberlist.Memberlist) string {
	var outStr string

	refTime := time.Now().UTC()

	var servers []*Server
	for _, svr := range state.Servers {
		servers = append(servers, svr)
	}

	sort.Sort(ServerByName(servers))

	outStr += "Services ------------------------------\n"
	for _, server := range servers {
		outStr += fmt.Sprintf("  %s: (%s)\n", server.Name, output.TimeAgo(server.LastUpdated, refTime))
		var services []*service.Service
		for _, svc := range server.Services {
			services = append(services, svc)
		}

		sort.Sort(ServicesByName(services))

		for _, svc := range ServicesByName(services) {
			outStr += svc.Format()
		}
		outStr += "\n"
	}

	// Don't show member list
	if list == nil {
		return outStr
	}

	outStr += "\nCluster Hosts -------------------------\n"
	for _, host := range list.Members() {
		outStr += fmt.Sprintf("    %s\n", host.Name)
	}

	outStr += "---------------------------------------"

	return outStr
}

// Print the formatted struct
func (state *ServicesState) Print(list *memberlist.Memberlist) {
	log.Println(state.Format(list))
}

// TrackNewServices talks to the discovery mechanism and tracks any services we
// don't already know about.
func (state *ServicesState) TrackNewServices(fn func() []service.Service, looper director.Looper) {
	looper.Loop(func() error {
		for _, svc := range fn() {
			state.UpdateService(svc)
		}
		return nil
	})
}

// TrackLocalListeners runs in the background and repeatedly calls
// a discovery function to return a list of event listeners. These will
// then be added to to the listener list. Managed listeners no longer
// reported from discovery will be removed.
func (state *ServicesState) TrackLocalListeners(fn func() []Listener, looper director.Looper) {
	looper.Loop(func() error {
		discovered := fn()
		// Add new listeners
		for _, listener := range discovered {
			state.RLock()
			_, ok := state.listeners[listener.Name()]
			state.RUnlock()

			if !ok {
				log.Infof("Adding listener %s because it was just discovered", listener.Name())
				urlListener, ok := listener.(*UrlListener)
				if ok {
					urlListener.Watch(state)
				} else {
					state.AddListener(listener)
				}
			}
		}
		// Remove old ones
		listeners := state.listeners
		for _, listener := range listeners {
			if listener.Managed() && !containsListener(discovered, listener.Name()) {
				log.Infof("Removing listener %s because the service appears to be gone", listener.Name())
				urlListener, ok := listener.(*UrlListener)
				if ok {
					log.Infof("Stopping UrlListener %s", listener.Name())
					urlListener.Stop()
				}
				err := state.RemoveListener(listener.Name())
				if err != nil {
					log.Warnf("Failed to remove listener %q: %s", listener.Name(), err)
				}
			}
		}

		return nil
	})
}

func containsListener(listeners []Listener, name string) bool {
	for _, listener := range listeners {
		if name == listener.Name() {
			return true
		}
	}
	return false
}

// Do we know about this service already? If we do, is it a tombstone?
func (state *ServicesState) IsNewService(svc *service.Service) bool {
	var found *service.Service

	if state.HasServer(svc.Hostname) {
		found = state.Servers[svc.Hostname].Services[svc.ID]
	}

	if found == nil || (!svc.IsTombstone() && svc.Status != found.Status) {
		return true
	}

	return false
}

// BroadcastServices loops forever, transmitting info about our containers on the
// broadcast channel. Intended to run as a background goroutine.
func (state *ServicesState) BroadcastServices(fn func() []service.Service, looper director.Looper) {
	lastTime := time.Unix(0, 0)

	looper.Loop(func() error {
		defer metrics.MeasureSince([]string{"services_state", "BroadcastServices"}, time.Now())
		var services []service.Service
		haveNewServices := false

		servicesList := fn()

		state.RLock()
		defer state.RUnlock()

		for _, svc := range servicesList {
			isNew := state.IsNewService(&svc)

			// We'll broadcast it now if it's new or we've hit refresh window
			if isNew {
				log.Debug("Found service changes in BroadcastServices()")
				haveNewServices = true
				services = append(services, svc)
				// Check that refresh window... is it time?
			} else if time.Now().UTC().Add(0 - ALIVE_BROADCAST_INTERVAL).After(lastTime) {
				services = append(services, svc)
			}
		}

		if len(services) > 0 {
			log.Debug("Starting to broadcast")
			// Figure out how many times to announce the service. New services get more announcements.
			runCount := 1
			if haveNewServices {
				runCount = ALIVE_COUNT
			}

			lastTime = time.Now().UTC()
			state.SendServices(
				services,
				director.NewTimedLooper(runCount, state.tombstoneRetransmit, nil),
			)
			log.Debug("Completing broadcast")
		} else {
			// We expect there to always be _something_ in the channel
			// once we've run.
			state.Broadcasts <- nil
		}

		return nil
	})
}

// Actually transmit an encoded service record into the channel. Runs a
// background goroutine that continues the broadcast for 10 seconds so we
// have a pretty good idea that it was delivered.
func (state *ServicesState) SendServices(services []service.Service, looper director.Looper) {
	// Announce these every second for awhile
	go func() {
		defer metrics.MeasureSince([]string{"services_state", "SendServices"}, time.Now())

		additionalTime := 0 * time.Second
		looper.Loop(func() error {
			var prepared [][]byte

			for _, svc := range services {
				svc.Updated = svc.Updated.Add(additionalTime)
				encoded, err := svc.Encode()
				if err != nil {
					log.Errorf("ERROR encoding container: (%s)", err.Error())
				}
				prepared = append(prepared, encoded)
			}

			// We add time to make sure that these get retransmitted by peers.
			// Otherwise they aren't "new" messages and don't get retransmitted.
			additionalTime = additionalTime + 50*time.Nanosecond
			state.Broadcasts <- prepared // Put it on the wire
			return nil
		})
	}()
}

func (state *ServicesState) BroadcastTombstones(fn func() []service.Service, looper director.Looper) {
	looper.Loop(func() error {
		defer metrics.MeasureSince([]string{"services_state", "BroadcastTombstones"}, time.Now())

		state.Lock()
		defer state.Unlock()

		containerList := fn()
		// Tell people about our dead services
		otherTombstones := state.TombstoneOthersServices()
		tombstones := state.TombstoneServices(state.Hostname, containerList)

		tombstones = append(tombstones, otherTombstones...)

		if len(tombstones) > 0 {
			state.SendServices(
				tombstones,
				director.NewTimedLooper(TOMBSTONE_COUNT, state.tombstoneRetransmit, nil),
			)
		} else {
			// We expect there to always be _something_ in the channel
			// once we've run.
			state.Broadcasts <- nil
		}

		return nil
	})
}

func (state *ServicesState) TombstoneOthersServices() []service.Service {
	defer metrics.MeasureSince([]string{"services_state", "TombstoneOthersServices"}, time.Now())

	var result []service.Service

	// Manage tombstone life so we don't keep them forever. We have to do this
	// even for hosts that aren't running services now, because they might have
	// been. Make sure we don't keep alive services around for very much
	// time at all.
	state.EachService(func(hostname *string, id *string, svc *service.Service) {
		if svc.IsTombstone() &&
			svc.Updated.Before(time.Now().UTC().Add(0-TOMBSTONE_LIFESPAN)) {
			delete(state.Servers[*hostname].Services, *id)

			// If this is the last service, remove the server
			if len(state.Servers[*hostname].Services) < 1 {
				delete(state.Servers, *hostname)
			}
		}

		svcLifespan := ALIVE_LIFESPAN
		if svc.IsDraining() {
			svcLifespan = DRAINING_LIFESPAN
		}
		// Everything that is not tombstoned needs to be considered for
		// removal if it exceeds the allowed ALIVE_TIMESPAN
		if !svc.IsTombstone() &&
			svc.Updated.Before(time.Now().UTC().Add(0-svcLifespan)) {
			log.Warnf("Found expired service %s ID %s from %s, tombstoning",
				svc.Name, svc.ID, svc.Hostname,
			)

			// Because we don't know that other hosts haven't gotten a newer
			// message that we missed, we'll tombstone them with the original
			// timestamp + 1 second. This way we don't invalidate newer records
			// we didn't see. This might happen when any node is removed from
			// cluster and re-joins, for example. So we can't use svc.Tombstone()
			// which updates the timestamp to Now().UTC()
			previousStatus := svc.Status
			svc.Status = service.TOMBSTONE
			svc.Updated = svc.Updated.Add(time.Second)
			state.ServiceChanged(svc, previousStatus, svc.Updated)

			result = append(result, *svc)
		}
	})

	return result
}

func (state *ServicesState) TombstoneServices(hostname string, containerList []service.Service) []service.Service {

	if !state.HasServer(hostname) {
		log.Debug("TombstoneServices(): New host or not running services, skipping.")
		return nil
	}
	// Build a map from the list first
	mapping := makeServiceMapping(containerList)

	var result []service.Service

	// Copy this so we can change the real list in the loop
	services := state.Servers[hostname].Services

	// Tombstone our own services that went away
	for id, svc := range services {
		if _, ok := mapping[id]; !ok && !svc.IsTombstone() {
			log.Warnf("Tombstoning %s", svc.ID)
			previousStatus := svc.Status
			svc.Tombstone()
			state.ServiceChanged(svc, previousStatus, svc.Updated)

			// Tombstone each record twice to help with receipt
			for i := 0; i < 2; i++ {
				result = append(result, *svc)
			}
		}
	}

	return result
}

func (state *ServicesState) EachServer(fn func(hostname *string, server *Server)) {
	if state == nil {
		return
	}
	for hostname, server := range state.Servers {
		fn(&hostname, server)
	}
}

func (state *ServicesState) EachService(fn func(hostname *string, serviceId *string, svc *service.Service)) {
	state.EachServer(func(hostname *string, server *Server) {
		services := server.Services

		for id, svc := range services {
			fn(hostname, &id, svc)
		}
	})
}

// Group the services into a map by service name rather than by the
// hosts they run on.
func (state *ServicesState) ByService() map[string][]*service.Service {
	serviceMap := make(map[string][]*service.Service)

	state.EachServiceSorted(
		func(hostname *string, serviceId *string, svc *service.Service) {
			serviceMap[svc.Name] = append(serviceMap[svc.Name], svc)
		},
	)

	return serviceMap
}

func DecodeStream(input io.Reader, callback func(map[string][]*service.Service, error)) error {
	dec := json.NewDecoder(input)
	for dec.More() {
		var conf map[string][]*service.Service
		err := dec.Decode(&conf)
		callback(conf, err)
		if err != nil {
			log.Errorf("Error decoding stream (%s)", err.Error())
			return err
		}
	}
	return nil
}

func makeServiceMapping(svcList []service.Service) map[string]*service.Service {
	mapping := make(map[string]*service.Service, len(svcList))
	for _, svc := range svcList {
		mapping[svc.ID] = &svc
	}

	return mapping
}

// Take a byte slice and return a properly reconstituted state struct
func Decode(data []byte) (*ServicesState, error) {
	newState := NewServicesState()
	err := newState.UnmarshalJSON(data)
	if err != nil {
		log.Errorf("Error decoding state! (%s)", err.Error())
	}

	return newState, err
}
