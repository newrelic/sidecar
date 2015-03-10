package main

import (
	"regexp"

	"github.com/BurntSushi/toml"
)

type HAproxyConfig struct {
	ReloadCmd string `toml:"reload_command"`
	VerifyCmd string `toml:"verify_command"`
	BindIP string `toml:"bind_ip"`
	TemplateFile string `toml:"template_file"`
	ConfigFile string `toml:"config_file"`
}

type ServicesConfig struct {
	NameMatch string `toml:"name_match"`
	NameRegexp *regexp.Regexp
}

type BosunConfig struct {
	ExcludeIPs []string `toml:"exclude_ips"`
}

type Config struct {
	Bosun BosunConfig
	Services ServicesConfig
	HAproxy HAproxyConfig
}

func parseConfig(path string) Config {
	var config Config

	_, err := toml.DecodeFile(path, &config)
	if err != nil {
		exitWithError(err, "Failed to parse config file")
	}

	config.Services.NameRegexp, err = regexp.Compile(config.Services.NameMatch)
	exitWithError(err, "Cant compile name_match regex")

	return config
}
