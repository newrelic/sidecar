package main

import (
	"errors"
	"flag"
	"log"
)

type CliOpts struct {
	ClusterIP	 string
}

func exitWithError(err error, message string) {
    if err != nil {
        log.Fatalf("%s (%s)", message, err.Error())
    }
}

func parseCommandLine() *CliOpts {
	var opts CliOpts

	flag.StringVar(&opts.ClusterIP, "cluster-ip", "", "The initial cluster bootstrap IP")

	flag.Parse()
	if opts.ClusterIP == "" {
		exitWithError(errors.New(""), "-cluster-ip is required!")
	}

	return &opts
}
