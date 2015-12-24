Docker Support
==============

This directory contains the basics to build and start a Docker container that
runs Sidecar. To work properly the container assumes that it is run in host
networking mode. The command line arguments required to run it on are in the
`run` script in this directory.

A few environment variables can be passed to the container:

```
SIDECAR_SEEDS="seed1 host2 seed3" # Required
ADVERTISE_IP="192.168.168.5" # Optional
```

The seed hosts passed via `SIDECAR_SEEDS` are formatted as
`--cluster-ip` arguments to Sidecar.

The default configuration is all set up with the expectation that you will map
`/var/run/docker.sock` into the container.  This is where Docker usually writes
its Unix socket. If you want to use TCP to connect, you'll need to do some more
work.
