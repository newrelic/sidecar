package main

import (
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
)

type HAproxyConfig struct {
	ReloadCmd    string `toml:"reload_command"`
	VerifyCmd    string `toml:"verify_command"`
	BindIP       string `toml:"bind_ip"`
	TemplateFile string `toml:"template_file"`
	ConfigFile   string `toml:"config_file"`
	PidFile      string `toml:"pid_file"`
	Disable      bool   `toml:"disable"`
}

type ServicesConfig struct {
	NameMatch  string `toml:"name_match"`
	NameRegexp *regexp.Regexp
}

type SidecarConfig struct {
	ExcludeIPs       []string `toml:"exclude_ips"`
	Discovery        []string `toml:"discovery"`
	StatsAddr        string   `toml:"stats_addr"`
	PushPullInterval duration `toml:"push_pull_interval"`
	GossipMessages   int      `toml:"gossip_messages"`
	TomeAddr         string   `toml:"tome_addr"`
}

type DockerConfig struct {
	DockerURL string `toml:"docker_url"`
}

type StaticConfig struct {
	ConfigFile string `toml:"config_file"`
}

type Config struct {
	Sidecar         SidecarConfig  `toml:"sidecar"`
	DockerDiscovery DockerConfig   `toml:"docker_discovery"`
	StaticDiscovery StaticConfig   `toml:"static_discovery"`
	Services        ServicesConfig `toml:"services"`
	HAproxy         HAproxyConfig  `toml:"haproxy"`
}

func setDefaults(config *Config) {
	config.DockerDiscovery.DockerURL = "tcp://localhost:2375"
	config.StaticDiscovery.ConfigFile = "static.json"
	config.Sidecar.TomeAddr = "localhost:7776"
}

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func parseConfig(path string) Config {
	var config Config

	setDefaults(&config)

	_, err := toml.DecodeFile(path, &config)
	if err != nil {
		exitWithError(err, "Failed to parse config file")
	}

	config.Services.NameRegexp, err = regexp.Compile(config.Services.NameMatch)
	exitWithError(err, "Cant compile name_match regex")

	return config
}
