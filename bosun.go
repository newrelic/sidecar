package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/newrelic/bosun/services_state"
	"github.com/newrelic/bosun/docker_discovery"
)

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

func announceMembers(list *memberlist.Memberlist, state *services_state.ServicesState) {
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
	opts     := parseCommandLine()
	state    := services_state.NewServicesState()
	delegate := servicesDelegate{state: state}

	// Use a LAN config but add our delegate
	config := memberlist.DefaultLANConfig()
	config.Delegate = &delegate
	config.Events   = &delegate

	log.Println("Bosun starting -------------------")
	log.Printf("Cluster Seeds: %s\n", strings.Join(*opts.ClusterIPs, ", "))
	log.Println("----------------------------------")

	list, err := memberlist.Create(config)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(*opts.ClusterIPs)
	exitWithError(err, "Failed to join cluster")

	metaUpdates := make(chan []byte)
	var wg sync.WaitGroup
	wg.Add(1)

	quitBroadcastingServices   := make(chan bool)
	quitBroadcastingTombstones := make(chan bool)
	quitDiscovery              := make(chan bool)

	docker := docker_discovery.New("tcp://localhost:2375")
	docker.Run(quitDiscovery)

	go announceMembers(list, state)
	go state.BroadcastServices(docker.Services, quitBroadcastingServices)
	go state.BroadcastTombstones(docker.Services, quitBroadcastingTombstones)
	go updateMetaData(list, metaUpdates)

	serveHttp(list, state)

	time.Sleep(4 * time.Second)
	metaUpdates <-[]byte("A message!")

	wg.Wait() // forever... nothing will decrement the wg
}
