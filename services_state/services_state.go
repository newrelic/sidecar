package services_state

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/newrelic/bosun/service"
)

const (
	TOMBSTONE_LIFESPAN = 3 * time.Hour
)

// Holds the state about one server in our cluster
type Server struct {
	Name string
	Services map[string]*service.Service
	LastUpdated time.Time
}

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
}

func NewServicesState() *ServicesState {
	var state ServicesState
	state.Servers = make(map[string]*Server, 5)
	return &state
}

func (state *ServicesState) Encode() []byte {
	jsonData, err := json.Marshal(state.Servers)
	if err != nil {
		log.Println("ERROR: Failed to Marshal state")
		return []byte{}
	}

	return jsonData
}

func (state *ServicesState) HasServer(hostname string) bool {
	if state.Servers[hostname] != nil {
		return true
	}

	return false
}

func (state *ServicesState) AddServiceEntry(data service.Service) {

	if !state.HasServer(data.Hostname) {
		state.Servers[data.Hostname] = NewServer(data.Hostname)
	}

	containerRef := state.Servers[data.Hostname]
	// Only apply changes that are newer
	if containerRef.Services[data.ID] == nil || data.Updated.After(containerRef.Services[data.ID].Updated) {
		containerRef.Services[data.ID] = &data
	}

	containerRef.LastUpdated = time.Now().UTC()
}

func (state *ServicesState) Format(list *memberlist.Memberlist) string {
	var output string

	output += "Services ------------------------------\n"
	for hostname, server := range state.Servers {
		output += fmt.Sprintf("  %s: (%s)\n", hostname, server.LastUpdated.String())
		for _, service := range server.Services {
			output += service.Format()
		}
		output += "\n"
	}

	output += "\nCluster Hosts -------------------------\n"
	for _, host := range list.Members() {
		output += fmt.Sprintf("    %s\n", host.Name)
	}

	output += "---------------------------------------"

	return output
}

func (state *ServicesState) Print(list *memberlist.Memberlist) {
	log.Println(state.Format(list))
}

// Loops forever, keeping our state current, and transmitting state
// on the broadcast channel. Intended to run as a background goroutine.
func (state *ServicesState) StayCurrent(broadcasts chan [][]byte, fn func() []service.Service ) {
	for ;; {
		containerList := fn()
		prepared := make([][]byte, len(containerList))

		for _, container := range containerList {
			state.AddServiceEntry(container)
			encoded, err := container.Encode()
			if err != nil {
				log.Printf("ERROR encoding container: (%s)", err.Error())
				continue
			}

			prepared = append(prepared, encoded)
		}

		// Tell people about our dead services
		tombstones := state.TombstoneServices(containerList)
		if tombstones != nil {
			prepared = append(prepared, tombstones...)
		}

		broadcasts <- prepared // Put it on the wire

		time.Sleep(2 * time.Second)
	}
}

func (state *ServicesState) TombstoneServices(containerList []service.Service) [][]byte {
	hostname, _ := os.Hostname()

	if !state.HasServer(hostname) {
		log.Println("TombstoneServices(): New host or not running services, skipping.")
		return nil
	}

	result := make([][]byte, len(containerList))

	// Build a map from the list first
	mapping := make(map[string]*service.Service, len(containerList))
	for _, container := range containerList {
		mapping[container.ID] = &container
	}

	for id, svc := range state.Servers[hostname].Services {
		if mapping[id] == nil && svc.Status == service.ALIVE {
			log.Printf("Tombstoning %s\n", svc.ID)
			svc.Tombstone()
			// Tombstone each record twice to help with receipt
			for i := 0; i < 2; i++ {
				encoded, err := svc.Encode()
				if err != nil {
					log.Printf("ERROR encoding container: (%s)", err.Error())
					continue
				}

				result = append(result, encoded)
			}
		}
	}

	return result
}
