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

Testing
-------

A good way to test out if your system is working, is to deploy the official
Nginx container and see if it shows up in discovery. The following command line
will do that:

```
$ docker run --label HealthCheck=HttpGet \ # Specify HttpGet health check type
	--label ServicePort_80=8080 \  # Map 8080 to exposed port 80
	--label ServicePort_443=8443 \ # Map 8443 to exposed port 443
	--label HealthCheckArgs="http://{{ host }}:{{ tcp 8080 }}/" \ # Health check this URL
	-d -P nginx                    # Detach and map ports
```
