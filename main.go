package main // import "github.com/Nitro/sidecar"

import (
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/armon/go-metrics"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/discovery"
	"github.com/Nitro/sidecar/haproxy"
	"github.com/Nitro/sidecar/healthy"
	"github.com/Nitro/sidecar/service"
	"github.com/Nitro/memberlist"
	"github.com/relistan/go-director"
)

var (
	profilerFile os.File
)

func announceMembers(list *memberlist.Memberlist, state *catalog.ServicesState) {
	for {
		// Ask for members of the cluster
		for _, member := range list.Members() {
			log.Debugf("Member: %s %s", member.Name, member.Addr)
			log.Debugf("Meta: %s", string(member.Meta))
		}

		state.RLock()
		log.Debug(state.Format(list))
		state.RUnlock()

		time.Sleep(2 * time.Second)
	}
}

func configureHAproxy(config Config) *haproxy.HAproxy {
	proxy := haproxy.New(config.HAproxy.ConfigFile, config.HAproxy.PidFile)

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

	if len(config.HAproxy.User) > 0 {
		proxy.User = config.HAproxy.User
	}

	if len(config.HAproxy.Group) > 0 {
		proxy.Group = config.HAproxy.Group
	}

	return proxy
}

func configureDiscovery(config *Config, opts *CliOpts) discovery.Discoverer {
	disco := new(discovery.MultiDiscovery)

	var svcNamer discovery.ServiceNamer
	var usingDocker bool
	var discoverers []string

	if opts.Discover != nil && len(*opts.Discover) > 0 {
		discoverers = *opts.Discover
	} else {
		discoverers = config.Sidecar.Discovery
	}

	for _, method := range discoverers {
		if method == "docker" {
			usingDocker = true
		}
	}

	switch config.Services.ServiceNamer {
	case "docker_label":
		svcNamer = &discovery.DockerLabelNamer{
			Label: config.Services.NameLabel,
		}
	case "regex":
		svcNamer = &discovery.RegexpNamer{
			ServiceNameMatch: config.Services.NameMatch,
		}
	default:
		if usingDocker {
			log.Fatalf("Unable to configure service namer! Not a valid entry.")
		}
	}

	for _, method := range discoverers {
		switch method {
		case "docker":
			disco.Discoverers = append(
				disco.Discoverers,
				discovery.NewDockerDiscovery(config.DockerDiscovery.DockerURL, svcNamer),
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
	if config.Sidecar.StatsAddr != "" {
		sink, err := metrics.NewStatsdSink(config.Sidecar.StatsAddr)
		exitWithError(err, "Can't configure Statsd")

		metricsConfig := metrics.DefaultConfig("sidecar")
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

	delegate.Start()

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

func configureLoggingLevel(level string) {
	switch {
	case len(level) == 0:
		log.SetLevel(log.InfoLevel)
	case level == "info":
		log.SetLevel(log.InfoLevel)
	case level == "warn":
		log.SetLevel(log.WarnLevel)
	case level == "error":
		log.SetLevel(log.ErrorLevel)
	case level == "debug":
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	opts := parseCommandLine()
	configureSignalHandler(opts)

	// Enable CPU profiling support if requested
	if *opts.CpuProfile {
		profilerFile, err := os.Create("sidecar.cpu.prof")
		exitWithError(err, "Can't write profiling file")
		pprof.StartCPUProfile(profilerFile)
		log.Debug("Profiling!")
	}

	// Create a new state instance and fire up the processor
	state := catalog.NewServicesState()
	svcMsgLooper := director.NewFreeLooper(
		director.FOREVER, make(chan error),
	)
	go state.ProcessServiceMsgs(svcMsgLooper)

	delegate := configureDelegate(state, opts)

	config := parseConfig(*opts.ConfigFile)

	// We can switch to JSON formatted logs from here on
	if config.Sidecar.LoggingFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		// Default to verbose timestamping
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}

	// Prefer loglevel from the CLI, then the Env, then the config
	if opts.LoggingLevel != nil {
		configureLoggingLevel(*opts.LoggingLevel)
	} else {
		configureLoggingLevel(config.Sidecar.LoggingLevel)
	}

	// Use a LAN config but add our delegate
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Delegate = delegate
	mlConfig.Events = delegate

	// Set some memberlist settings
	mlConfig.LogOutput = &LoggingBridge{}
	mlConfig.PreferTCPDNS = false

	// Set up the push pull interval for Memberlist
	if config.Sidecar.PushPullInterval.Duration == 0 {
		mlConfig.PushPullInterval = catalog.ALIVE_LIFESPAN - 1*time.Second
	} else {
		mlConfig.PushPullInterval = config.Sidecar.PushPullInterval.Duration
	}
	if config.Sidecar.GossipMessages != 0 {
		mlConfig.GossipMessages = config.Sidecar.GossipMessages
	}

	// Make sure we pass on the cluster name to Memberlist
	mlConfig.ClusterName = *opts.ClusterName

	// Figure out our IP address from the CLI or by inspecting
	publishedIP, err := getPublishedIP(config.Sidecar.ExcludeIPs, opts.AdvertiseIP)
	exitWithError(err, "Failed to find private IP address")
	mlConfig.AdvertiseAddr = publishedIP

	log.Println("Sidecar starting -------------------")
	log.Printf("Cluster Name: %s", *opts.ClusterName)
	log.Printf("Config File: %s", *opts.ConfigFile)
	log.Printf("Cluster Seeds: %s", strings.Join(*opts.ClusterIPs, ", "))
	log.Printf("Advertised address: %s", publishedIP)
	log.Printf("Service Name Match: %s", config.Services.NameMatch)
	log.Printf("Excluded IPs: %v", config.Sidecar.ExcludeIPs)
	log.Printf("Push/Pull Interval: %s", config.Sidecar.PushPullInterval.Duration.String())
	log.Printf("Gossip Messages: %d", config.Sidecar.GossipMessages)
	log.Printf("Logging level: %s", config.Sidecar.LoggingLevel)
	log.Printf("Running HAproxy: %t", !config.HAproxy.Disable && !*opts.HAproxyDisable)
	log.Println("----------------------------------")

	list, err := memberlist.Create(mlConfig)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(*opts.ClusterIPs)
	exitWithError(err, "Failed to join cluster")

	servicesLooper := director.NewTimedLooper(
		director.FOREVER, catalog.ALIVE_SLEEP_INTERVAL, nil,
	)
	tombstoneLooper := director.NewTimedLooper(
		director.FOREVER, catalog.TOMBSTONE_SLEEP_INTERVAL, nil,
	)
	trackingLooper := director.NewTimedLooper(
		director.FOREVER, catalog.ALIVE_SLEEP_INTERVAL, nil,
	)
	discoLooper := director.NewTimedLooper(
		director.FOREVER, discovery.SLEEP_INTERVAL, make(chan error),
	)
	healthWatchLooper := director.NewTimedLooper(
		director.FOREVER, healthy.WATCH_INTERVAL, make(chan error),
	)
	healthLooper := director.NewTimedLooper(
		director.FOREVER, healthy.HEALTH_INTERVAL, make(chan error),
	)

	configureMetrics(&config)

	// Register the cluster name with the state object
	state.ClusterName = *opts.ClusterName

	disco := configureDiscovery(&config, opts)
	go disco.Run(discoLooper)

	// Configure the monitor and use the public address as the default
	// check address.
	monitor := healthy.NewMonitor(publishedIP, config.Sidecar.DefaultCheckEndpoint)

	// Wrap the monitor Services function as a simple func without the receiver
	serviceFunc := func() []service.Service { return monitor.Services() }

	// Need to call HAproxy first, otherwise won't see first events from
	// discovered services, and then won't write them out.
	var proxy *haproxy.HAproxy

	if !*opts.HAproxyDisable && !config.HAproxy.Disable {
		proxy = configureHAproxy(config)
		go proxy.Watch(state)
	}

	// If we have any callback Urls for state change notifications, let's
	// put them here.
	for _, url := range config.Listeners.Urls {
		listener := catalog.NewUrlListener(url)
		listener.Watch(state)
	}

	go announceMembers(list, state)
	go state.BroadcastServices(serviceFunc, servicesLooper)
	go state.BroadcastTombstones(serviceFunc, tombstoneLooper)
	go state.TrackNewServices(serviceFunc, trackingLooper)
	go monitor.Watch(disco, healthWatchLooper)
	go monitor.Run(healthLooper)

	go serveHttp(list, state)

	if !*opts.HAproxyDisable && !config.HAproxy.Disable {
		proxy.WriteAndReload(state)
	}

	select {}
}
