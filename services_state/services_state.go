package services_state

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/newrelic/bosun/output"
	"github.com/newrelic/bosun/service"
)

// services_state handles all of the eventual-consistency mechanisms for
// service discovery state. The ServicesState struct has a mapping of
// servers to Service lists and manages the lifecycle.

const (
	TOMBSTONE_LIFESPAN = 3 * time.Hour          // How long we keep tombstones around
	TOMBSTONE_COUNT = 10                        // Send tombstones at 1 per second 10 times
	TOMBSTONE_SLEEP_INTERVAL = 2 * time.Second  // Sleep between local service checks
	ALIVE_LIFESPAN = 20 * time.Second           // Down if not heard from in 20 seconds
	ALIVE_SLEEP_INTERVAL = 2 * time.Second      // Sleep between local service checks
)

// Holds the state about one server in our cluster
type Server struct {
	Name string
	Services map[string]*service.Service
	LastUpdated time.Time
}

// Returns a pointer to a properly configured Server
func NewServer(name string) *Server {
	var server Server
	server.Name = name
	// Pre-create for 5 services per host
	server.Services = make(map[string]*service.Service, 5)
	server.LastUpdated = time.Unix(0, 0)

	return &server
}

// Holds the state about all the servers in the cluster
type ServicesState struct {
	Servers map[string]*Server
	HostnameFn func() (string, error)
	Broadcasts chan [][]byte
}

// Returns a pointer to a properly configured ServicesState
func NewServicesState() *ServicesState {
	var state ServicesState
	state.Servers    = make(map[string]*Server, 5)
	state.HostnameFn = os.Hostname
	state.Broadcasts = make(chan [][]byte)
	return &state
}

// Return a Marshaled/Encoded byte array that can be deocoded with
// services_state.Decode()
func (state *ServicesState) Encode() []byte {
	jsonData, err := json.Marshal(state.Servers)
	if err != nil {
		log.Println("ERROR: Failed to Marshal state")
		return []byte{}
	}

	return jsonData
}

// Shortcut for checking if the Servers map has an entry for this
// hostname.
func (state *ServicesState) HasServer(hostname string) bool {
	if state.Servers[hostname] != nil {
		return true
	}

	return false
}

// A server has left the cluster, so tombstone all of its records
func (state *ServicesState) ExpireServer(hostname string, quit chan bool) {
	if !state.HasServer(hostname) {
		log.Printf("No records to expire for %s\n", hostname)
		return
	}

	log.Printf("Expiring %s\n", hostname)

	tombstones := make([][]byte, 0, len(state.Servers[hostname].Services))

	for _, svc := range state.Servers[hostname].Services {
		svc.Tombstone()

		encoded, err := svc.Encode()
		if err != nil {
			log.Printf("ERROR encoding service: (%s)", err.Error())
			continue
		}

		tombstones = append(tombstones, encoded)
	}

	state.SendTombstones(tombstones, quit)
}

// Take a service and merge it into our state. Correctly handle
// timestamps so we only add things newer than what we already
// know about.
func (state *ServicesState) AddServiceEntry(entry service.Service) {

	if !state.HasServer(entry.Hostname) {
		state.Servers[entry.Hostname] = NewServer(entry.Hostname)
	}

	server := state.Servers[entry.Hostname]
	// Only apply changes that are newer
	if server.Services[entry.ID] == nil ||
			entry.Invalidates(server.Services[entry.ID]) {
		server.Services[entry.ID] = &entry
		server.LastUpdated = entry.Updated
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

// Loops forever, keeping transmitting info about our containers
// on the broadcast channel. Intended to run as a background goroutine.
func (state *ServicesState) BroadcastServices(fn func() []service.Service, quit chan bool) {
	lastTime := time.Now().UTC()

	for ;; {
		containerList := fn()
		var prepared [][]byte

		for _, container := range containerList {
			state.AddServiceEntry(container)
			encoded, err := container.Encode()
			if err != nil {
				log.Printf("ERROR encoding container: (%s)", err.Error())
				continue
			}

			lastTime = container.Updated
			prepared = append(prepared, encoded)
		}

		log.Printf("Starting to broadcast")
		if len(prepared) > 0 {
			state.Broadcasts <- prepared // Put it on the wire
		} else {
			// We expect there to always be _something_ in the channel
			// once we've run.
			state.Broadcasts <- nil
		}
		log.Printf("Completing broadcast")

		// Now that we're finished, see if we're supposed to exit
		select {
			case <- quit:
				return
			default:
		}

		timeDiff := time.Now().UTC().Sub(lastTime)

		if timeDiff < ALIVE_SLEEP_INTERVAL {
			log.Printf("Sleeping %v", timeDiff)
			time.Sleep(ALIVE_SLEEP_INTERVAL - timeDiff)
		}
	}
}

// Actually transmit an encoded tombstone record into the channel. Runs a
// background goroutine that continues the broadcast for 10 seconds so we
// have a pretty good idea that it was delivered.
func (state *ServicesState) SendTombstones(tombstones [][]byte, quit chan bool) {
	state.Broadcasts <- tombstones // Put it on the wire

	// Announce these every second for awhile
	go func() {
		for i := 0; i < TOMBSTONE_COUNT; i++ {
			state.Broadcasts <- tombstones

			select {
			case <- quit:
				return
			default:
			}

			time.Sleep(1 * time.Second)
		}
	}()
}

func (state *ServicesState) BroadcastTombstones(fn func() []service.Service, quit chan bool) {
	hostname, _ := state.HostnameFn()
	propagateQuit := make(chan bool)

	for ;; {
		containerList := fn()
		// Tell people about our dead services
		otherTombstones := state.TombstoneOthersServices()
		tombstones      := state.TombstoneServices(hostname, containerList)

		tombstones = append(tombstones, otherTombstones...)

		if tombstones != nil && len(tombstones) > 0 {
			state.SendTombstones(tombstones, propagateQuit)
		} else {
			// We expect there to always be _something_ in the channel
			// once we've run.
			state.Broadcasts <- nil
		}

		// Now that we're finished, see if we're supposed to exit
		select {
			case <- quit:
				propagateQuit <- true
				return
			default:
		}

		time.Sleep(TOMBSTONE_SLEEP_INTERVAL)
	}
}

func (state *ServicesState) TombstoneOthersServices() [][]byte {
	result := make([][]byte, 0, 1)

	// Manage tombstone life so we don't keep them forever. We have to do this
	// even for hosts that aren't running services now, because they might have
	// been. Make sure we don't keep alive services around for very much
	// time at all.
	state.EachService(func(hostname *string, id *string, svc *service.Service) {
		if svc.IsTombstone() &&
				svc.Updated.Before(time.Now().UTC().Add(0 - TOMBSTONE_LIFESPAN)) {
			delete(state.Servers[*hostname].Services, *id)
			// If this is the last service, remove the server
			if len(state.Servers[*hostname].Services) < 1 {
				delete(state.Servers, *hostname)
			}
		}

		if svc.IsAlive() &&
				svc.Updated.Before(time.Now().UTC().Add(0 - ALIVE_LIFESPAN)) {

			log.Printf("Found expired service %s from %s, tombstoning",
				svc.Name, svc.Hostname,
			)

			// Because we don't know that other hosts haven't gotten a newer
			// message that we missed, we'll tombstone them with the original
			// timestamp + 1 second. This way we don't invalidate newer records
			// we didn't see. This might happen when any node is removed from
			// cluster and re-joins, for example. So we can't use svc.Tombstone()
			// which updates the timestamp to Now().UTC()
			svc.Status = service.TOMBSTONE
			svc.Updated = svc.Updated.Add(time.Second)

			encoded, err := svc.Encode()

			if err != nil {
				log.Printf("ERROR encoding container: (%s)", err.Error())
			}

			result = append(result, encoded)
		}
	})

	return result
}

func (state *ServicesState) TombstoneServices(hostname string, containerList []service.Service) [][]byte {

	if !state.HasServer(hostname) {
		println("TombstoneServices(): New host or not running services, skipping.")
		return nil
	}
	// Build a map from the list first
	mapping := makeServiceMapping(containerList)

	result := make([][]byte, 0, len(containerList))

	// Copy this so we can change the real list in the loop
	services := state.Servers[hostname].Services

	// Tombstone our own services that went away
	for id, svc := range services {
		if mapping[id] == nil && svc.Status == service.ALIVE {
			log.Printf("Tombstoning %s\n", svc.ID)
			svc.Tombstone()
			// Tombstone each record twice to help with receipt
			encoded, err := svc.Encode()
			if err != nil {
				log.Printf("ERROR encoding container: (%s)", err.Error())
			}

			for i := 0; i < 2; i++ {
				result = append(result, encoded)
			}
		}
	}

	return result
}

func (state *ServicesState) EachService(fn func(hostname *string, serviceId *string, svc *service.Service)) {
	for hostname, _ := range state.Servers {
		for id, svc := range state.Servers[hostname].Services {
			fn(&hostname, &id, svc)
		}
	}
}

func (state *ServicesState) ByService() map[string][]*service.Service {
	serviceMap := make(map[string][]*service.Service)

	state.EachServiceSorted(
		func(hostname *string, serviceId *string, svc *service.Service) {
			if _, ok := serviceMap[svc.Image]; !ok {
				serviceMap[svc.Image] = make([]*service.Service, 0, 3)
			}
			serviceMap[svc.Image] = append(serviceMap[svc.Image], svc)
		},
	)

	return serviceMap
}

func makeServiceMapping(containerList []service.Service) map[string]*service.Service {
	mapping := make(map[string]*service.Service, len(containerList))
	for _, container := range containerList {
		mapping[container.ID] = &container
	}

	return mapping
}

// Take a byte slice and return a properly reconstituted state struct
func Decode(data []byte) (*ServicesState, error) {
	newState := NewServicesState()
	err := json.Unmarshal(data, &newState.Servers)
	if err != nil {
		log.Printf("Error decoding state! (%s)", err.Error())
	}

	return newState, err
}
