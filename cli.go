package main

import (
	"log"

	"gopkg.in/alecthomas/kingpin.v1"
)

type CliOpts struct {
	ClusterIPs *[]string
}

func exitWithError(err error, message string) {
    if err != nil {
        log.Fatalf("%s (%s)", message, err.Error())
    }
}

func parseCommandLine() *CliOpts {
	var opts CliOpts

	opts.ClusterIPs = kingpin.Flag("cluster-ip", "The cluster seed addresses").Required().Short('c').Strings()
	kingpin.Parse()

	return &opts
}
