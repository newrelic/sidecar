package main

import (
	"time"

	"github.com/kelseyhightower/envconfig"
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
	UseHostnames bool   `envconfig:"USE_HOSTNAMES" default:"false"`
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

func parseConfig(path string) Config {
	var config Config

	err := envconfig.Process("sidecar", &config.Sidecar)
	if err != nil {
		exitWithError(err, "Failed to parse environment config")
	}

	err = envconfig.Process("docker", &config.DockerDiscovery)
	if err != nil {
		exitWithError(err, "Failed to parse environment config")
	}

	err = envconfig.Process("static", &config.StaticDiscovery)
	if err != nil {
		exitWithError(err, "Failed to parse environment config")
	}

	err = envconfig.Process("services", &config.Services)
	if err != nil {
		exitWithError(err, "Failed to parse environment config")
	}

	err = envconfig.Process("haproxy", &config.HAproxy)
	if err != nil {
		exitWithError(err, "Failed to parse environment config")
	}

	err = envconfig.Process("listeners", &config.Listeners)
	if err != nil {
		exitWithError(err, "Failed to parse environment config")
	}

	return config
}
