package main // import "github.com/newrelic/sidecar"

import (
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/armon/go-metrics"
	"github.com/newrelic-forks/memberlist"
	"github.com/newrelic/sidecar/catalog"
	"github.com/newrelic/sidecar/discovery"
	"github.com/newrelic/sidecar/haproxy"
	"github.com/newrelic/sidecar/healthy"
	"github.com/newrelic/sidecar/service"
	"github.com/relistan/go-director"
)

var (
	profilerFile os.File
)

func announceMembers(list *memberlist.Memberlist, state *catalog.ServicesState) {
	for {
		// Ask for members of the cluster
		for _, member := range list.Members() {
			log.Printf("Member: %s %s", member.Name, member.Addr)
			log.Printf("Meta: %s", string(member.Meta))
		}

		log.Debug(state.Format(list))

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

func configureDiscovery(config *Config) discovery.Discoverer {
	disco := new(discovery.MultiDiscovery)

	for _, method := range config.Sidecar.Discovery {
		switch method {
		case "docker":
			disco.Discoverers = append(
				disco.Discoverers,
				discovery.NewDockerDiscovery(
					config.DockerDiscovery.DockerURL,
					config.DockerDiscovery.NameFromLabel,
					config.DockerDiscovery.NameMatch,
					config.DockerDiscovery.NameRegexp,
				),
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
		profilerFile, err := os.Create("sidecar.cpu.prof")
		exitWithError(err, "Can't write profiling file")
		pprof.StartCPUProfile(profilerFile)
		log.Debug("Profiling!")
	}
	config := parseConfig(*opts.ConfigFile)
	state := catalog.NewServicesState()
	delegate := configureDelegate(state, opts)

	// We can switch to JSON formatted logs from here on
	if config.Sidecar.LoggingFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		// Default to verbose timestamping
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}

	// Use a LAN config but add our delegate
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Delegate = delegate
	mlConfig.Events = delegate

	// Set up the push pull interval for Memberlist
	if config.Sidecar.PushPullInterval.Duration == 0 {
		mlConfig.PushPullInterval = catalog.ALIVE_LIFESPAN - 1*time.Second
	} else {
		mlConfig.PushPullInterval = config.Sidecar.PushPullInterval.Duration
	}
	if config.Sidecar.GossipMessages != 0 {
		mlConfig.GossipMessages = config.Sidecar.GossipMessages
	}

	// Figure out our IP address from the CLI or by inspecting
	publishedIP, err := getPublishedIP(config.Sidecar.ExcludeIPs, opts.AdvertiseIP)
	exitWithError(err, "Failed to find private IP address")
	mlConfig.AdvertiseAddr = publishedIP

	log.WithFields(log.Fields{
		"fields":            "main()",
		"clusterName":       *opts.ClusterName,
		"configFile":        *opts.ConfigFile,
		"seeds":             strings.Join(*opts.ClusterIPs, ", "),
		"advertisedAddress": publishedIP,
		"excludedIPs":       config.Sidecar.ExcludeIPs,
		"pushPullInterval":  config.Sidecar.PushPullInterval.Duration.String(),
		"gossip":            config.Sidecar.GossipMessages,
	})

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

	disco := configureDiscovery(&config)
	go disco.Run(discoLooper)

	// Configure the monitor and use the public address as the default
	// check address.
	monitor := healthy.NewMonitor(publishedIP)
	serviceFunc := func() []service.Service { return monitor.Services() }

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
	go monitor.Watch(disco, healthWatchLooper)
	go monitor.Run(healthLooper)

	if !config.HAproxy.Disable {
		proxy.WriteAndReload(state)
	}

	serveHttp(list, state)

	select {}
}
