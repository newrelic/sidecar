package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/discovery"
	"github.com/newrelic/bosun/haproxy"
	"github.com/newrelic/bosun/services_state"
)

func updateMetaData(list *memberlist.Memberlist, metaUpdates chan []byte) {
	for {
		list.LocalNode().Meta = <-metaUpdates // Blocking
		fmt.Printf("Got update: %s\n", string(list.LocalNode().Meta))
		err := list.UpdateNode(10 * time.Second)
		if err != nil {
			fmt.Printf("Error pushing node update!")
		}
	}
}

func announceMembers(list *memberlist.Memberlist, state *services_state.ServicesState) {
	for {
		// Ask for members of the cluster
		for _, member := range list.Members() {
			fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
			fmt.Printf("  Meta:\n    %s\n", string(member.Meta))
		}

		state.Print(list)

		time.Sleep(2 * time.Second)
	}
}

func configureHAproxy(config Config) *haproxy.HAproxy {
	proxy := haproxy.New()
	if len(config.HAproxy.BindIP) > 0 {
		proxy.BindIP = config.HAproxy.BindIP
	}

	if len(config.HAproxy.ReloadCmd) > 0 {
		proxy.ReloadCmd = config.HAproxy.ReloadCmd
	}

	if len(config.HAproxy.VerifyCmd) > 0 {
		proxy.VerifyCmd = config.HAproxy.VerifyCmd
	}

	if len(config.HAproxy.TemplateFile) > 0 {
		proxy.Template = config.HAproxy.TemplateFile
	}

	if len(config.HAproxy.ConfigFile) > 0 {
		proxy.ConfigFile = config.HAproxy.ConfigFile
	}

	return proxy
}

func main() {
	opts := parseCommandLine()
	state := services_state.NewServicesState()
	delegate := NewServicesDelegate(state)

	config := parseConfig("bosun.toml")
	state.ServiceNameMatch = config.Services.NameRegexp

	// Use a LAN config but add our delegate
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Delegate = delegate
	mlConfig.Events = delegate

	publishedIP, err := getPublishedIP(config.Bosun.ExcludeIPs)
	exitWithError(err, "Failed to find private IP address")
	mlConfig.AdvertiseAddr = publishedIP

	log.Println("Bosun starting -------------------")
	log.Printf("Cluster Seeds: %s\n", strings.Join(*opts.ClusterIPs, ", "))
	log.Printf("Advertised address: %s\n", publishedIP)
	log.Printf("Service Name Match: %s\n", config.Services.NameMatch)
	log.Printf("Excluded IPs: %v\n", config.Bosun.ExcludeIPs)
	log.Println("----------------------------------")

	list, err := memberlist.Create(mlConfig)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(*opts.ClusterIPs)
	exitWithError(err, "Failed to join cluster")

	metaUpdates := make(chan []byte)
	var wg sync.WaitGroup
	wg.Add(1)

	quitDiscovery := make(chan bool)
	servicesLooper := director.NewTimedLooper(
		director.FOREVER, services_state.ALIVE_SLEEP_INTERVAL, nil,
	)
	tombstoneLooper := director.NewTimedLooper(
		director.FOREVER, services_state.TOMBSTONE_SLEEP_INTERVAL, nil,
	)

	docker := discovery.NewDockerDiscovery("tcp://localhost:2375")
	docker.Run(quitDiscovery)

	go announceMembers(list, state)
	go state.BroadcastServices(docker.Services, servicesLooper)
	go state.BroadcastTombstones(docker.Services, tombstoneLooper)
	go updateMetaData(list, metaUpdates)

	if !config.HAproxy.Disable {
		proxy := configureHAproxy(config)
		go proxy.Watch(state)
	}

	serveHttp(list, state)

	time.Sleep(4 * time.Second)
	metaUpdates <- []byte("A message!")

	wg.Wait() // forever... nothing will decrement the wg
}
