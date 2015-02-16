package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

var broadcasts chan [][]byte
var servicesState map[string]*Server

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

type servicesDelegate struct {}

func (d *servicesDelegate) NodeMeta(limit int) []byte {
	fmt.Printf("NodeMeta(): %d\n", limit)
	return []byte(`{ "State": "Running" }`)
}

func (d *servicesDelegate) NotifyMsg(message []byte) {
	if len(message) <  1 {
		fmt.Println("NotifyMsg(): empty")
		return
	}

	fmt.Printf("NotifyMsg(): %s\n", string(message))

	// TODO don't just send container structs, send message structs
	data := Decode(message)
	if data == nil {
		fmt.Printf("NotifyMsg(): error decoding!\n")
		return
	}

	addServiceEntry(*data)
}

func (d *servicesDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	fmt.Printf("GetBroadcasts(): %d %d\n", overhead, limit)

	select {
		case broadcast := <-broadcasts:
			println("Sending broadcast")
			return broadcast
		default:
			return nil
	}
}

func (d *servicesDelegate) LocalState(join bool) []byte {
	fmt.Printf("LocalState(): %b\n", join)
	return []byte("some state")
}

func (d *servicesDelegate) MergeRemoteState(buf []byte, join bool) {
	fmt.Printf("MergeRemoteState(): %s %b\n", string(buf), join)
}

func updateState() {
	for ;; {
		containerList := containers()
		prepared := make([][]byte, len(containerList))

		for _, container := range containerList {
			addServiceEntry(container)
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

func announceMembers(list *memberlist.Memberlist) {
	for ;; {
		// Ask for members of the cluster
		for _, member := range list.Members() {
		    fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
			fmt.Printf("  Meta:\n    %s\n", string(member.Meta))
		}

		printServices(list);

		time.Sleep(2 * time.Second)
	}
}

func formatServices(list *memberlist.Memberlist) string {
	var output string

	output += "Services ------------------------------\n"
	for hostname, server := range servicesState {
		output += fmt.Sprintf("  %s: (%s)\n", hostname, server.LastUpdated.String())
		for _, service := range server.Services {
			output += fmt.Sprintf("      %s %-20s %-30s %s\n",
				service.ID,
				service.Name,
				service.Image,
				service.Created,
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

func printServices(list *memberlist.Memberlist) {
	println(formatServices(list))
}

func addServiceEntry(data ServiceContainer) {
	// Lazily create the maps
	if servicesState == nil {
		// Pre-create for 5 hosts
		servicesState = make(map[string]*Server, 5)
	}
	if servicesState[data.Hostname] == nil {
		var server Server
		server.Init(data.Hostname)
		servicesState[data.Hostname] = &server
	}

	servicesState[data.Hostname].Services[data.ID] = &data
	// TODO Use the message time!
	servicesState[data.Hostname].LastUpdated = time.Now().UTC()
}

func main() {
	opts := parseCommandLine()

	var delegate servicesDelegate

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

	go announceMembers(list)
	go updateState()
	go updateMetaData(list, metaUpdates)

	serveHttp(list)

	time.Sleep(4 * time.Second)
	metaUpdates <-[]byte("A message!")

	wg.Wait() // forever... nothing will decrement the wg
}
