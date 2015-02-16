package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

const (
	TOMBSTONE_LIFESPAN = 3 * time.Hour
)

var (
	broadcasts chan [][]byte
)

// Holds the state about one server in our cluster
type Server struct {
	Name string
	Services map[string]*Service
	LastUpdated time.Time
}

func (p *Server) Init(name string) {
	p.Name = ""
	// Pre-create for 5 services per host
	p.Services = make(map[string]*Service, 5)
	p.LastUpdated = time.Unix(0, 0)
}

// Holds the state about all the servers in the cluster
type ServicesState struct {
	Servers map[string]*Server
}

func (state *ServicesState) Init() {
	state.Servers = make(map[string]*Server, 5)
}

func (state *ServicesState) Encode() []byte {
	jsonData, err := json.Marshal(state.Servers)
	if err != nil {
		log.Println("ERROR: Failed to Marshal state")
		return []byte{}
	}

	return jsonData
}

func (state *ServicesState) HasEntry(hostname string) bool {
	if state.Servers[hostname] != nil {
		return true
	}

	return false
}

func (state *ServicesState) AddServiceEntry(data Service) {

	if !state.HasEntry(data.Hostname) {
		var server Server
		server.Init(data.Hostname)
		state.Servers[data.Hostname] = &server
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
func (state *ServicesState) StayCurrent() {
	for ;; {
		containerList := containers()
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

func (state *ServicesState) TombstoneServices(containerList []Service) [][]byte {
	hostname, _ := os.Hostname()

	if !state.HasEntry(hostname) {
		log.Println("TombstoneServices(): New host or not running services, skipping.")
		return nil
	}

	result := make([][]byte, len(containerList))

	// Build a map from the list first
	mapping := make(map[string]*Service, len(containerList))
	for _, container := range containerList {
		mapping[container.ID] = &container
	}

	for id, service := range state.Servers[hostname].Services {
		if mapping[id] == nil && service.Status == ALIVE {
			log.Printf("Tombstoning %s\n", service.ID)
			service.Tombstone()
			encoded, err := service.Encode()
			if err != nil {
				log.Printf("ERROR encoding container: (%s)", err.Error())
				continue
			}

			result = append(result, encoded)
		}
	}

	return result
}

func updateMetaData(list *memberlist.Memberlist, metaUpdates chan []byte) {
	for ;; {
		list.LocalNode().Meta = <-metaUpdates // Blocking
		fmt.Printf("Got update: %s\n", string(list.LocalNode().Meta))
		err := list.UpdateNode(10 * time.Second)
		if err != nil {
			fmt.Printf("Error pushing node update!")
		}
	}
}

func announceMembers(list *memberlist.Memberlist, state *ServicesState) {
	for ;; {
		// Ask for members of the cluster
		for _, member := range list.Members() {
		    fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
			fmt.Printf("  Meta:\n    %s\n", string(member.Meta))
		}

		state.Print(list);

		time.Sleep(2 * time.Second)
	}
}

func main() {
	opts := parseCommandLine()

	var state ServicesState
	state.Init()
	delegate := servicesDelegate{state: &state}

	broadcasts = make(chan [][]byte)

	// Use a LAN config but add our delegate
	config := memberlist.DefaultLANConfig()
	config.Delegate = &delegate

	list, err := memberlist.Create(config)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join([]string{ opts.ClusterIP })
	exitWithError(err, "Failed to join cluster")

	metaUpdates := make(chan []byte)
	var wg sync.WaitGroup
	wg.Add(1)

	go announceMembers(list, &state)
	go state.StayCurrent()
	go updateMetaData(list, metaUpdates)

	serveHttp(list, &state)

	time.Sleep(4 * time.Second)
	metaUpdates <-[]byte("A message!")

	wg.Wait() // forever... nothing will decrement the wg
}
