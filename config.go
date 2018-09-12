package main

import (
	"time"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/relistan/rubberneck.v1"
)

type ListenerUrlsConfig struct {
	Urls []string `envconfig:"URLS"`
}

type HAproxyConfig struct {
	ReloadCmd    string `envconfig:"RELOAD_COMMAND"`
	VerifyCmd    string `envconfig:"VERIFY_COMMAND"`
	BindIP       string `envconfig:"BIND_IP" default:"192.168.168.168"`
	TemplateFile string `envconfig:"TEMPLATE_FILE" default:"views/haproxy.cfg"`
	ConfigFile   string `envconfig:"CONFIG_FILE" default:"/etc/haproxy.cfg"`
	PidFile      string `envconfig:"PID_FILE" default:"/var/run/haproxy.pid"`
	Disable      bool   `envconfig:"DISABLE"`
	User         string `envconfig:"USER" default:"haproxy"`
	Group        string `envconfig:"GROUP" default:"haproxy"`
	UseHostnames bool   `envconfig:"USE_HOSTNAMES"`
}

type ServicesConfig struct {
	NameMatch    string `envconfig:"NAME_MATCH"`
	ServiceNamer string `envconfig:"NAMER" default:"docker_label"`
	NameLabel    string `envconfig:"NAME_LABEL" default:"ServiceName"`
}

type SidecarConfig struct {
	ExcludeIPs           []string      `envconfig:"EXCLUDE_IPS" default:"192.168.168.168"`
	Discovery            []string      `envconfig:"DISCOVERY" default:"docker"`
	StatsAddr            string        `envconfig:"STATS_ADDR"`
	PushPullInterval     time.Duration `envconfig:"PUSH_PULL_INTERVAL" default:"20s"`
	GossipMessages       int           `envconfig:"GOSSIP_MESSAGES" default:"15"`
	LoggingFormat        string        `envconfig:"LOGGING_FORMAT"`
	LoggingLevel         string        `envconfig:"LOGGING_LEVEL" default:"info"`
	DefaultCheckEndpoint string        `envconfig:"DEFAULT_CHECK_ENDPOINT" default:"/version"`
	Seeds                []string      `envconfig:"SEEDS"`
	ClusterName          string        `envconfig:"CLUSTER_NAME" default:"default"`
	AdvertiseIP          string        `envconfig:"ADVERTISE_IP"`
	BindPort             int           `envconfig:"BIND_PORT" default:"7946"`
}

type DockerConfig struct {
	DockerURL string `envconfig:"URL" default:"unix:///var/run/docker.sock"`
}

type StaticConfig struct {
	ConfigFile string `envconfig:"CONFIG_FILE" default:"static.json"`
}

type Config struct {
	Sidecar         SidecarConfig      // SIDECAR_
	DockerDiscovery DockerConfig       // DOCKER_
	StaticDiscovery StaticConfig       // STATIC_
	Services        ServicesConfig     // SERVICES_
	HAproxy         HAproxyConfig      // HAPROXY_
	Listeners       ListenerUrlsConfig // LISTENERS_
}

func parseConfig() Config {
	var config Config

	errs := []error{
		envconfig.Process("sidecar", &config.Sidecar),
		envconfig.Process("docker", &config.DockerDiscovery),
		envconfig.Process("static", &config.StaticDiscovery),
		envconfig.Process("services", &config.Services),
		envconfig.Process("haproxy", &config.HAproxy),
		envconfig.Process("listeners", &config.Listeners),
	}

	for _, err := range errs {
		if err != nil {
			rubberneck.Print(config)
			exitWithError(err, "Can't parse environment config!")
		}
	}

	return config
}
