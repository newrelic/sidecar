#!/bin/bash

die() {
	echo $1
	exit 1
}

file ../sidecar | grep "ELF.*LSB" || die "../sidecar is missing or not a Linux binary"
cp sidecar.docker.toml sidecar.toml
cp ../sidecar . && cp -pr ../views . && docker build . || die "Failed to build"
