package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

type CliOpts struct {
	AdvertiseIP    *string
	ClusterIPs     *[]string
	ClusterName    *string
	CpuProfile     *bool
	Discover       *[]string
	HAproxyDisable *bool
	LoggingLevel   *string
}

func exitWithError(err error, message string) {
	if err != nil {
		log.Fatalf("%s (%s)", message, err.Error())
	}
}

func parseCommandLine() *CliOpts {
	var opts CliOpts

	app := kingpin.New("sidecar", "")

	opts.AdvertiseIP = app.Flag("advertise-ip", "The address to advertise to the cluster").Short('a').String()
	opts.ClusterIPs = app.Flag("cluster-ip", "The cluster seed addresses").Required().Short('c').NoEnvar().Strings()
	opts.ClusterName = app.Flag("cluster-name", "The cluster we're part of").Short('n').String()
	opts.CpuProfile = app.Flag("cpuprofile", "Enable CPU profiling").Short('p').Bool()
	opts.Discover = app.Flag("discover", "Method of discovery").Short('d').NoEnvar().Strings()
	opts.HAproxyDisable = app.Flag("haproxy-disable", "Disable managing HAproxy").Short('x').Bool()
	opts.LoggingLevel = app.Flag("logging-level", "Set the logging level").Short('l').String()

	app.Parse(os.Args[1:])

	return &opts
}
