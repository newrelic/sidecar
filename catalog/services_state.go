package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/armon/go-metrics"
	"github.com/newrelic/sidecar/output"
	"github.com/newrelic/sidecar/service"
	"github.com/nitro/memberlist"
	"github.com/relistan/go-director"
)

// catalog handles all of the eventual-consistency mechanisms for
// service discovery state. The ServicesState struct has a mapping of
// servers to Service lists and manages the lifecycle.

const (
	TOMBSTONE_LIFESPAN       = 3 * time.Hour                  // How long we keep tombstones around
	TOMBSTONE_COUNT          = 10                             // Send tombstones at 1 per second 10 times
	ALIVE_COUNT              = 5                              // Send new services at 1 per second 5 times
	TOMBSTONE_SLEEP_INTERVAL = 2 * time.Second                // Sleep between local service checks
	TOMBSTONE_RETRANSMIT     = 1 * time.Second                // Time between tombstone retranmission
	ALIVE_LIFESPAN           = 1*time.Minute + 20*time.Second // Down if not heard from in 80 seconds
	ALIVE_SLEEP_INTERVAL     = 1 * time.Second                // Sleep between local service checks
	ALIVE_BROADCAST_INTERVAL = 1 * time.Minute                // Broadcast Alive messages every minute
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
	var server Server
	server.Name = name
	// Pre-create for 5 services per host
	server.Services = make(map[string]*service.Service, 5)
	server.LastUpdated = time.Unix(0, 0)
	server.LastChanged = time.Unix(0, 0)

	return &server
}

// Holds the state about all the servers in the cluster
type ServicesState struct {
	Servers             map[string]*Server
	LastChanged         time.Time
	Hostname            string             `json:"-"`
	Broadcasts          chan [][]byte      `json:"-"`
	listeners           []chan ChangeEvent `json:"-"`
	listenerLock        sync.Mutex         `json:"-"`
	tombstoneRetransmit time.Duration      `json:"-"`
	sync.Mutex
}

// Returns a pointer to a properly configured ServicesState
func NewServicesState() *ServicesState {
	var state ServicesState
	var err error
	state.Servers = make(map[string]*Server, 5)
	state.Broadcasts = make(chan [][]byte)
	state.LastChanged = time.Unix(0, 0)
	state.Hostname, err = os.Hostname()
	if err != nil {
		log.Errorf("Error getting hostname! %s", err.Error())
	}
	state.tombstoneRetransmit = TOMBSTONE_RETRANSMIT
	return &state
}

// Shortcut for checking if the server has this service or not.
func (server *Server) HasService(id string) bool {
	_, ok := server.Services[id]
	return ok
}

// Return a Marshaled/Encoded byte array that can be deocoded with
// catalog.Decode()
func (state *ServicesState) Encode() []byte {
	jsonData, err := json.Marshal(state)
	if err != nil {
		log.Error("ERROR: Failed to Marshal state")
		return []byte{}
	}

	return jsonData
}

// Shortcut for checking if the Servers map has an entry for this
// hostname.
func (state *ServicesState) HasServer(hostname string) bool {
	_, ok := state.Servers[hostname]
	return ok
}

// Looks up a service from *only this host* by ID
func (state *ServicesState) GetLocalService(id string) *service.Service {
	if !state.HasServer(state.Hostname) {
		// This can happen a lot on startup, so we're not logging it
		return nil
	}

	if state.Servers != nil &&
		state.Servers[state.Hostname] != nil &&
		state.Servers[state.Hostname].Services != nil {

		return state.Servers[state.Hostname].Services[id]
	}

	return nil
}

// A server has left the cluster, so tombstone all of its records
func (state *ServicesState) ExpireServer(hostname string) {
	if !state.HasServer(hostname) {
		log.Infof("No records to expire for %s", hostname)
		return
	}

	log.Infof("Expiring %s", hostname)

	tombstones := make([]service.Service, 0, len(state.Servers[hostname].Services))

	for _, svc := range state.Servers[hostname].Services {
		previousStatus := svc.Status
		state.ServiceChanged(svc, previousStatus, svc.Updated)
		svc.Tombstone()
		tombstones = append(tombstones, *svc)
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
func (state *ServicesState) serverChanged(hostname string, updated time.Time) {
	if !state.HasServer(hostname) {
		log.Errorf("Attempt to change a server we don't have! (%s)", hostname)
		return
	}

	state.Lock()
	state.Servers[hostname].LastUpdated = updated
	state.Servers[hostname].LastChanged = updated
	state.LastChanged = updated
	state.Unlock()
}

// Tell all of our listeners that something changed for a host at
// set timestamp. See AddListener() for information about how channels
// must be configured.
func (state *ServicesState) NotifyListeners(svc *service.Service, previousStatus int, changedTime time.Time) {
	if len(state.listeners) < 1 {
		log.Debugf("Skipping listeners, there are none")
		return
	}

	log.Infof("Notifying listeners of change at %s", changedTime.String())

	event := ChangeEvent{Service: *svc, PreviousStatus: previousStatus, Time: changedTime}
	state.listenerLock.Lock()
	for _, listener := range state.listeners {
		select {
		case listener <- event:
			continue
		default:
			log.Error("NotifyListeners(): Can't send to listener!")
		}
	}
	state.listenerLock.Unlock()
}

// Add an event listener channel to the list that will be notified on
// major state change events. Channels must be buffered by at least 1
// or they will block. Channels must be ready to receive input.
func (state *ServicesState) AddListener(listener chan ChangeEvent) {
	state.listenerLock.Lock()
	state.listeners = append(state.listeners, listener)
	state.listenerLock.Unlock()
	log.Debugf("AddListener(): new count %d", len(state.listeners))
}

// Take a service and merge it into our state. Correctly handle
// timestamps so we only add things newer than what we already
// know about. Retransmits updates to cluster peers.
func (state *ServicesState) AddServiceEntry(entry service.Service) {
	defer metrics.MeasureSince([]string{"services_state", "AddServiceEntry"}, time.Now())

	if !state.HasServer(entry.Hostname) {
		state.Servers[entry.Hostname] = NewServer(entry.Hostname)
	}

	server := state.Servers[entry.Hostname]
	// Only apply changes that are newer or services are missing
	if !server.HasService(entry.ID) {
		server.Services[entry.ID] = &entry
		state.ServiceChanged(&entry, service.UNKNOWN, entry.Updated)
		state.retransmit(entry)
	} else if entry.Invalidates(server.Services[entry.ID]) {
		server.LastUpdated = entry.Updated
		if server.Services[entry.ID].Status != entry.Status {
			state.ServiceChanged(&entry, server.Services[entry.ID].Status, entry.Updated)
		}
		server.Services[entry.ID] = &entry
		// We tell our gossip peers about the updated service
		// by sending them the record. We're saved from an endless
		// retransmit loop by the Invalidates() call above.
		state.retransmit(entry)
		return
	}
}

// Merge a complete state struct into this one. Usually used on
// node startup and during anti-entropy operations.
func (state *ServicesState) Merge(otherState *ServicesState) {
	for _, server := range otherState.Servers {
		for _, service := range server.Services {
			state.AddServiceEntry(*service)
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

	outStr += "Services ------------------------------\n"
	for hostname, server := range state.Servers {
		outStr += fmt.Sprintf("  %s: (%s)\n", hostname, output.TimeAgo(server.LastUpdated, refTime))
		for _, service := range server.Services {
			outStr += service.Format()
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

// Talk to the discovery mechanism and track any services we don't
// already know about.
func (state *ServicesState) TrackNewServices(fn func() []service.Service, looper director.Looper) {
	looper.Loop(func() error {
		for _, container := range fn() {
			state.AddServiceEntry(container)
		}
		return nil
	})
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

// Loops forever, transmitting info about our containers on the
// broadcast channel. Intended to run as a background goroutine.
func (state *ServicesState) BroadcastServices(fn func() []service.Service, looper director.Looper) {
	lastTime := time.Unix(0, 0)

	looper.Loop(func() error {
		defer metrics.MeasureSince([]string{"services_state", "BroadcastServices"}, time.Now())
		var services []service.Service
		haveNewServices := false

		servicesList := fn()

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
		metrics.MeasureSince([]string{"services_state", "SendServices"}, time.Now())

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
		metrics.MeasureSince([]string{"services_state", "BroadcastTombstones"}, time.Now())

		containerList := fn()
		// Tell people about our dead services
		otherTombstones := state.TombstoneOthersServices()
		tombstones := state.TombstoneServices(state.Hostname, containerList)

		tombstones = append(tombstones, otherTombstones...)

		if tombstones != nil && len(tombstones) > 0 {
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
	metrics.MeasureSince([]string{"services_state", "TombstoneOthersServices"}, time.Now())

	result := make([]service.Service, 0, 1)

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

		if svc.IsAlive() &&
			svc.Updated.Before(time.Now().UTC().Add(0-ALIVE_LIFESPAN)) {

			log.Warnf("Found expired service %s from %s, tombstoning",
				svc.Name, svc.Hostname,
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

	result := make([]service.Service, 0, len(containerList))

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
	for hostname, server := range state.Servers {
		fn(&hostname, server)
	}
}

func (state *ServicesState) EachService(fn func(hostname *string, serviceId *string, svc *service.Service)) {
	state.EachServer(func(hostname *string, server *Server) {
		for id, svc := range server.Services {
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
			if _, ok := serviceMap[svc.Name]; !ok {
				serviceMap[svc.Name] = make([]*service.Service, 0, 3)
			}
			serviceMap[svc.Name] = append(serviceMap[svc.Name], svc)
		},
	)

	return serviceMap
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
	err := json.Unmarshal(data, &newState)
	if err != nil {
		log.Errorf("Error decoding state! (%s)", err.Error())
	}

	return newState, err
}
