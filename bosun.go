package main // import "github.com/newrelic/bosun"

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	"github.com/newrelic-forks/memberlist"
	"github.com/relistan/go-director"
	"github.com/newrelic/bosun/catalog"
	"github.com/newrelic/bosun/discovery"
	"github.com/newrelic/bosun/haproxy"
)

var (
	profilerFile os.File
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

func announceMembers(list *memberlist.Memberlist, state *catalog.ServicesState) {
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

func configureDiscovery(config *Config) discovery.Discoverer {
	disco := new(discovery.MultiDiscovery)

	for _, method := range config.Bosun.Discovery {
		switch method {
		case "docker":
			disco.Discoverers = append(
				disco.Discoverers,
				discovery.NewDockerDiscovery(config.DockerDiscovery.DockerURL),
			)
		case "static":
			disco.Discoverers = append(
				disco.Discoverers,
				discovery.NewStaticDiscovery(config.StaticDiscovery.ConfigFile),
			)
		default:
		}
	}

	return disco
}

func configureMetrics(config *Config) {
	if config.Bosun.StatsAddr != "" {
		sink, err := metrics.NewStatsdSink(config.Bosun.StatsAddr)
		exitWithError(err, "Can't configure Statsd")

		metricsConfig := metrics.DefaultConfig("bosun")
		_, err = metrics.NewGlobal(metricsConfig, sink)
		exitWithError(err, "Can't start metrics")
	}
}

func configureDelegate(state *catalog.ServicesState, opts *CliOpts) *servicesDelegate {
	delegate := NewServicesDelegate(state)
	delegate.Metadata = NodeMetadata{
		ClusterName: *opts.ClusterName,
		State:       "Running",
	}

	return delegate
}

func configureSignalHandler(opts *CliOpts) {
	if !*opts.CpuProfile {
		return
	}

	// Capture CTRL-C and stop the CPU profiler                            
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, os.Interrupt)
	go func() {
		for sig := range sigChannel {
			log.Printf("Captured %v, stopping profiler and exiting..", sig)
			pprof.StopCPUProfile()
			profilerFile.Close()
			os.Exit(1)
		}
	}()
}

func main() {
	opts := parseCommandLine()
	configureSignalHandler(opts)
	// Enable CPU profiling support if requested
	if *opts.CpuProfile {
		profilerFile, err := os.Create("bosun.cpu.prof")
		exitWithError(err, "Can't write profiling file")
		pprof.StartCPUProfile(profilerFile)
	}
	state := catalog.NewServicesState()
	delegate := configureDelegate(state, opts)

	config := parseConfig(*opts.ConfigFile)

	state.ServiceNameMatch = config.Services.NameRegexp

	// Use a LAN config but add our delegate
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Delegate = delegate
	mlConfig.Events = delegate

	// Set up the push pull interval for Memberlist
	if config.Bosun.PushPullInterval.Duration == 0 {
		mlConfig.PushPullInterval = catalog.ALIVE_LIFESPAN - 1*time.Second
	} else {
		mlConfig.PushPullInterval = config.Bosun.PushPullInterval.Duration
	}
	if config.Bosun.GossipMessages != 0 {
		mlConfig.GossipMessages = config.Bosun.GossipMessages
	}

	// Figure out our IP address from the CLI or by inspecting
	publishedIP, err := getPublishedIP(config.Bosun.ExcludeIPs, opts.AdvertiseIP)
	exitWithError(err, "Failed to find private IP address")
	mlConfig.AdvertiseAddr = publishedIP

	log.Println("Bosun starting -------------------")
	log.Printf("Cluster Name: %s\n", *opts.ClusterName)
	log.Printf("Config File: %s\n", *opts.ConfigFile)
	log.Printf("Cluster Seeds: %s\n", strings.Join(*opts.ClusterIPs, ", "))
	log.Printf("Advertised address: %s\n", publishedIP)
	log.Printf("Service Name Match: %s\n", config.Services.NameMatch)
	log.Printf("Excluded IPs: %v\n", config.Bosun.ExcludeIPs)
	log.Printf("Push/Pull Interval: %s\n", config.Bosun.PushPullInterval.Duration.String())
	log.Printf("Gossip Messages: %d\n", config.Bosun.GossipMessages)
	log.Println("----------------------------------")

	list, err := memberlist.Create(mlConfig)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(*opts.ClusterIPs)
	exitWithError(err, "Failed to join cluster")

	//metaUpdates := make(chan []byte)

	quitDiscovery := make(chan bool)

	servicesLooper := director.NewTimedLooper(
		director.FOREVER, catalog.ALIVE_SLEEP_INTERVAL, nil,
	)
	tombstoneLooper := director.NewTimedLooper(
		director.FOREVER, catalog.TOMBSTONE_SLEEP_INTERVAL, nil,
	)
	trackingLooper := director.NewTimedLooper(
		director.FOREVER, catalog.ALIVE_SLEEP_INTERVAL, nil,
	)

	configureMetrics(&config)

	disco := configureDiscovery(&config)
	disco.Run(quitDiscovery)

	go announceMembers(list, state)
	go state.BroadcastServices(disco.Services, servicesLooper)
	go state.BroadcastTombstones(disco.Services, tombstoneLooper)
	go state.TrackNewServices(disco.Services, trackingLooper)
	//go updateMetaData(list, metaUpdates)

	if !config.HAproxy.Disable {
		proxy := configureHAproxy(config)
		go proxy.Watch(state)
	}

	serveHttp(list, state)

	time.Sleep(4 * time.Second)
	//metaUpdates <- []byte("A message!")

	select {}
}
