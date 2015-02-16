package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

var (
	broadcasts chan [][]byte
)

type ServicesState struct {
	Servers map[string]*Server
}

func (state *ServicesState) Init() {
	state.Servers = make(map[string]*Server, 5)
}

func (state *ServicesState) HasEntry(hostname string) bool {
	if state.Servers[hostname] != nil {
		return true
	}

	return false
}

func (state *ServicesState) AddServiceEntry(data ServiceContainer) {

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
			output += fmt.Sprintf("      %s %-20s %-30s %20s\n",
				service.ID,
				service.Name,
				service.Image,
				service.Updated,
			)
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
	println(state.Format(list))
}

type Server struct {
	Name string
	Services map[string]*ServiceContainer
	LastUpdated time.Time
}

func (p *Server) Init(name string) {
	p.Name = ""
	// Pre-create for 5 services per host
	p.Services = make(map[string]*ServiceContainer, 5)
	p.LastUpdated = time.Unix(0, 0)
}

func updateState(state *ServicesState) {
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
		broadcasts <- prepared

		time.Sleep(2 * time.Second)
	}
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
	go updateState(&state)
	go updateMetaData(list, metaUpdates)

	serveHttp(list, &state)

	time.Sleep(4 * time.Second)
	metaUpdates <-[]byte("A message!")

	wg.Wait() // forever... nothing will decrement the wg
}
