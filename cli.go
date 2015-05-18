package main

import (
	log "github.com/Sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v1"
)

type CliOpts struct {
	AdvertiseIP *string
	ClusterIPs  *[]string
	ConfigFile  *string
	ClusterName *string
	CpuProfile  *bool
}

func exitWithError(err error, message string) {
	if err != nil {
		log.Fatalf("%s (%s)", message, err.Error())
	}
}

func parseCommandLine() *CliOpts {
	var opts CliOpts

	opts.AdvertiseIP = kingpin.Flag("advertise-ip", "The address to advertise to the cluster").Short('a').String()
	opts.ClusterIPs = kingpin.Flag("cluster-ip", "The cluster seed addresses").Required().Short('c').Strings()
	opts.ConfigFile = kingpin.Flag("config-file", "The config file to use").Short('f').Default("bosun.toml").String()
	opts.ClusterName = kingpin.Flag("cluster-name", "The cluster we're part of").Short('n').Default("default").String()
	opts.CpuProfile = kingpin.Flag("cpuprofile", "Enable CPU profiling").Short('p').Bool()
	kingpin.Parse()

	return &opts
}
