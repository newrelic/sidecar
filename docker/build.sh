#!/bin/bash

die() {
	echo $1
	exit 1
}

file ../sidecar | grep "ELF.*LSB" || die "../sidecar is missing or not a Linux binary"
test -f sidecar.toml || cp sidecar.docker.toml sidecar.toml
echo "Building..."
cd .. && docker build -f docker/Dockerfile -t sidecar . || die "Failed to build"
