package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/newrelic/bosun/services_state"
)

var (
	broadcasts chan [][]byte
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

	broadcasts = make(chan [][]byte)

	// Use a LAN config but add our delegate
	config := memberlist.DefaultWANConfig()
	config.Delegate = &delegate

	list, err := memberlist.Create(config)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join([]string{ opts.ClusterIP })
	exitWithError(err, "Failed to join cluster")

	metaUpdates := make(chan []byte)
	var wg sync.WaitGroup
	wg.Add(1)

	go announceMembers(list, state)
	go state.StayCurrent(broadcasts, containers)
	go updateMetaData(list, metaUpdates)

	serveHttp(list, state)

	time.Sleep(4 * time.Second)
	metaUpdates <-[]byte("A message!")

	wg.Wait() // forever... nothing will decrement the wg
}
