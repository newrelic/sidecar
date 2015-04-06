package main

import (
	"log"

	"gopkg.in/alecthomas/kingpin.v1"
)

type CliOpts struct {
	ClusterIPs *[]string
	ConfigFile *string
}

func exitWithError(err error, message string) {
    if err != nil {
        log.Fatalf("%s (%s)", message, err.Error())
    }
}

func parseCommandLine() *CliOpts {
	var opts CliOpts

	opts.ClusterIPs = kingpin.Flag("cluster-ip",  "The cluster seed addresses").Required().Short('c').Strings()
	opts.ConfigFile = kingpin.Flag("config-file", "The config file to use").Short('f').Default("bosun.toml").String()
	kingpin.Parse()

	return &opts
}
