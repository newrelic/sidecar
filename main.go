package main // import "github.com/Nitro/sidecar"

import (
	"context"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	"github.com/Nitro/memberlist"
	"github.com/Nitro/sidecar/catalog"
	"github.com/Nitro/sidecar/config"
	"github.com/Nitro/sidecar/discovery"
	"github.com/Nitro/sidecar/envoy"
	"github.com/Nitro/sidecar/haproxy"
	"github.com/Nitro/sidecar/healthy"
	"github.com/Nitro/sidecar/service"
	"github.com/Nitro/sidecar/sidecarhttp"
	"github.com/armon/go-metrics"
	"github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
	"gopkg.in/relistan/rubberneck.v1"
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

// configureOverrides takes CLI opts and applies them over the top of settings
// taken from the environment variables and stored in config.
func configureOverrides(config *config.Config, opts *CliOpts) {
	if len(*opts.AdvertiseIP) > 0 {
		config.Sidecar.AdvertiseIP = *opts.AdvertiseIP
	}
	if len(*opts.ClusterIPs) > 0 {
		config.Sidecar.Seeds = *opts.ClusterIPs
	}
	if len(*opts.ClusterName) > 0 {
		config.Sidecar.ClusterName = *opts.ClusterName
	}
	if len(*opts.Discover) > 0 {
		config.Sidecar.Discovery = *opts.Discover
	}
	if len(*opts.LoggingLevel) > 0 {
		config.Sidecar.LoggingLevel = *opts.LoggingLevel
	}
}

func configureHAproxy(config *config.Config) *haproxy.HAproxy {
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

	proxy.UseHostnames = config.HAproxy.UseHostnames

	return proxy
}

func configureDiscovery(config *config.Config, publishedIP string) discovery.Discoverer {
	disco := new(discovery.MultiDiscovery)

	var svcNamer discovery.ServiceNamer
	var usingDocker bool
	var err error

	if len(config.Sidecar.Discovery) < 1 {
		log.Warn("No discovery method configured! Sidecar running in passive mode")
	}

	for _, method := range config.Sidecar.Discovery {
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
		svcNamer, err = discovery.NewRegexpNamer(config.Services.NameMatch)
		if err != nil {
			log.Fatalf("Unable to use RegexpNamer: %s", err)
		}
	default:
		if usingDocker {
			log.Fatalf("Unable to configure service namer! Not a valid entry.")
		}
	}

	for _, method := range config.Sidecar.Discovery {
		switch method {
		case "docker":
			disco.Discoverers = append(
				disco.Discoverers,
				discovery.NewDockerDiscovery(config.DockerDiscovery.DockerURL, svcNamer, publishedIP),
			)
		case "static":
			disco.Discoverers = append(
				disco.Discoverers,
				discovery.NewStaticDiscovery(config.StaticDiscovery.ConfigFile, publishedIP),
			)
		default:
		}
	}

	return disco
}

// configureMetrics sets up remote performance metrics if we're asked to send them (statsd)
func configureMetrics(config *config.Config) {
	if config.Sidecar.StatsAddr != "" {
		sink, err := metrics.NewStatsdSink(config.Sidecar.StatsAddr)
		exitWithError(err, "Can't configure Statsd")

		metricsConfig := metrics.DefaultConfig("sidecar")
		_, err = metrics.NewGlobal(metricsConfig, sink)
		exitWithError(err, "Can't start metrics")
	}
}

// configureDelegate sets up the Memberlist delegate we'll use
func configureDelegate(state *catalog.ServicesState, config *config.Config) *servicesDelegate {
	delegate := NewServicesDelegate(state)
	delegate.Metadata = NodeMetadata{
		ClusterName: config.Sidecar.ClusterName,
		State:       "Running",
	}

	delegate.Start()

	return delegate
}

// configureCpuProfiler sets of the CPU profiler and a signal handler to
// stop it if we have been told to run the CPU profiler.
func configureCpuProfiler(opts *CliOpts) {
	if !*opts.CpuProfile {
		return
	}

	var profilerFile os.File

	// Capture CTRL-C and stop the CPU profiler
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, os.Interrupt)
	go func() {
		for sig := range sigChannel {
			log.Printf("Captured %v, stopping profiler and exiting..", sig)
			pprof.StopCPUProfile()
			profilerFile.Close()
			os.Exit(0)
		}
	}()

	// Enable CPU profiling support if requested
	if *opts.CpuProfile {
		profilerFile, err := os.Create("sidecar.cpu.prof")
		exitWithError(err, "Can't write profiling file")
		err = pprof.StartCPUProfile(profilerFile)
		exitWithError(err, "Can't start the CPU profiler")
		log.Debug("Profiling!")
	}
}

func configureLoggingLevel(config *config.Config) {
	level := config.Sidecar.LoggingLevel

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

// configureLoggingFormat switches between text and JSON log format
func configureLoggingFormat(config *config.Config) {
	if config.Sidecar.LoggingFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		// Default to verbose timestamping
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}
}

func configureMemberlist(config *config.Config, state *catalog.ServicesState) *memberlist.Config {
	delegate := configureDelegate(state, config)

	// Use a LAN config but add our delegate
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Delegate = delegate
	mlConfig.Events = delegate

	// Set some memberlist settings
	mlConfig.LogOutput = &LoggingBridge{} // Use logrus as backend for Memberlist
	mlConfig.PreferTCPDNS = false

	// Set up the push pull interval for Memberlist
	if config.Sidecar.PushPullInterval == 0 {
		mlConfig.PushPullInterval = catalog.ALIVE_LIFESPAN - 1*time.Second
	} else {
		mlConfig.PushPullInterval = config.Sidecar.PushPullInterval
	}
	if config.Sidecar.GossipMessages != 0 {
		mlConfig.GossipMessages = config.Sidecar.GossipMessages
	}

	// Make sure we pass on the cluster name to Memberlist
	mlConfig.ClusterName = config.Sidecar.ClusterName

	// Figure out our IP address from the CLI or by inspecting the network interfaces
	publishedIP, err := getPublishedIP(config.Sidecar.ExcludeIPs, config.Sidecar.AdvertiseIP)
	exitWithError(err, "Failed to find private IP address")
	mlConfig.BindPort = config.Sidecar.BindPort
	mlConfig.AdvertiseAddr = publishedIP
	mlConfig.AdvertisePort = config.Sidecar.BindPort

	return mlConfig
}

// configureListeners sets up any statically configured state change event listeners.
func configureListeners(config *config.Config, state *catalog.ServicesState) {
	for _, url := range config.Listeners.Urls {
		listener := catalog.NewUrlListener(url, false)
		listener.Watch(state)
	}
}

func main() {
	config := config.ParseConfig()
	opts := parseCommandLine()
	configureOverrides(config, opts)
	configureCpuProfiler(opts)
	configureLoggingLevel(config)
	configureLoggingFormat(config)
	configureMetrics(config)

	// Create a new state instance and fire up the processor. We need
	// this to happen early in the startup.
	state := catalog.NewServicesState()
	svcMsgLooper := director.NewFreeLooper(
		director.FOREVER, make(chan error),
	)
	go state.ProcessServiceMsgs(svcMsgLooper)

	configureListeners(config, state)

	mlConfig := configureMemberlist(config, state)

	printer := rubberneck.NewPrinter(log.Infof, rubberneck.NoAddLineFeed)
	printer.PrintWithLabel("Sidecar", config)

	list, err := memberlist.Create(mlConfig)
	exitWithError(err, "Failed to create memberlist")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(config.Sidecar.Seeds)
	exitWithError(err, "Failed to join cluster")

	// Set up a bunch of go-director Loopers to run our
	// background goroutines
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
		director.FOREVER, discovery.DefaultSleepInterval, make(chan error),
	)
	listenLooper := director.NewTimedLooper(
		director.FOREVER, discovery.DefaultSleepInterval, make(chan error),
	)
	healthWatchLooper := director.NewTimedLooper(
		director.FOREVER, healthy.WATCH_INTERVAL, make(chan error),
	)
	healthLooper := director.NewTimedLooper(
		director.FOREVER, healthy.HEALTH_INTERVAL, make(chan error),
	)

	// Register the cluster name with the state object
	state.ClusterName = config.Sidecar.ClusterName

	disco := configureDiscovery(config, mlConfig.AdvertiseAddr)
	go disco.Run(discoLooper)

	// Configure the monitor and use the public address as the default
	// check address.
	monitor := healthy.NewMonitor(mlConfig.AdvertiseAddr, config.Sidecar.DefaultCheckEndpoint)

	// Wrap the monitor Services function as a simple func without the receiver
	serviceFunc := func() []service.Service { return monitor.Services() }

	// Wrap the discovery Listeners output in something the state can handle
	listenFunc := func() []catalog.Listener {
		listeners := disco.Listeners()
		var result []catalog.Listener
		for _, discovered := range listeners {
			newLstnr := catalog.NewUrlListener(discovered.Url, true)
			newLstnr.SetName(discovered.Name)
			result = append(result, newLstnr)
		}
		return result
	}

	// Need to call HAproxy first, otherwise won't see first events from
	// discovered services, and then won't write them out.
	var proxy *haproxy.HAproxy

	if !config.HAproxy.Disable {
		proxy = configureHAproxy(config)
		go proxy.Watch(state)
	}

	go announceMembers(list, state)
	go state.BroadcastServices(serviceFunc, servicesLooper)
	go state.BroadcastTombstones(serviceFunc, tombstoneLooper)
	go state.TrackNewServices(serviceFunc, trackingLooper)
	go state.TrackLocalListeners(listenFunc, listenLooper)
	go monitor.Watch(disco, healthWatchLooper)
	go monitor.Run(healthLooper)

	go sidecarhttp.ServeHttp(list, state, &sidecarhttp.HttpConfig{
		BindIP:       config.HAproxy.BindIP,
		UseHostnames: config.HAproxy.UseHostnames,
	})

	if !config.HAproxy.Disable {
		err := proxy.WriteAndReload(state)
		exitWithError(err, "Failed to reload HAProxy config")
	}

	if config.Envoy.UseGRPCAPI {
		ctx := context.Background()
		envoyServer := envoy.NewServer(ctx, state, config.Envoy)
		envoyServerLooper := director.NewFreeLooper(
			director.FOREVER, make(chan error),
		)

		// This listener will be owned and managed by the gRPC server
		grpcListener, err := net.Listen("tcp", ":"+config.Envoy.GRPCPort)
		if err != nil {
			log.Fatalf("Failed to listen on port %q: %s", config.Envoy.GRPCPort, err)
		}

		go envoyServer.Run(ctx, envoyServerLooper, grpcListener)

		state.AddListener(envoyServer.Listener)
	}

	select {}
}
