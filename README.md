Sidecar ![Sidecar](views/static/Sidecar.png)
=====

[![](https://travis-ci.org/Nitro/sidecar.svg?branch=master)](https://travis-ci.org/Nitro/sidecar)
[![](https://images.microbadger.com/badges/image/gonitro/sidecar.svg)](https://microbadger.com/images/gonitro/sidecar)
[![](https://images.microbadger.com/badges/version/gonitro/sidecar.svg)](https://microbadger.com/images/gonitro/sidecar "Get your own version badge on microbadger.com")

**The main repo for this project is the [Nitro
fork](https://github.com/Nitro/sidecar)**

Sidecar is a dynamic service discovery platform requiring no external
coordination service. It's a peer-to-peer system that uses a gossip protocol
for all communication between hosts. Sidecar health checks local services and
announces them to peer systems. It's Docker-native so your containerized
applications work out of the box. It's designed to be **A**vailable,
**P**artition tolerant, and eventually consistent—where "eventually" is a very
short time window on the matter of a few seconds.

Sidecar is part of a small ecosystem of tools. It can stand entirely alone
or can also leverage:

 * [haproxy-api](https://github.com/Nitro/haproxy-api) - A separation layer
   that allows Sidecar and HAproxy to run in seperate containers. It also
   allows a local HAproxy to be configured against a remote Sidecar instance.

 * [Superside](https://github.com/Nitro/superside) - A multi-environment
   console for Sidecar. Has a heads up display, event lists, and graphs
   showing what is happening in one or more Sidecar clusters on a live
   basis.

 * [sidecar-executor](https://github.com/Nitro/sidecar-executor) - A Mesos
   executor that integrates with Sidecar, allowing your containers to be
   health checked by Sidecar for both service health and service discovery.
   Also supports a number of extra features including Vault integration for
   secrets management.

 * [sidecar-dns](https://github.com/relistan/sidecar-dns) - a WIP project
   to serve DNS SRV records from Sidecar services state.

 * [Traefik plugin](https://github.com/Nitro/traefik) - A fork of Traefik
   that can be backed by Sidecar. Useful as a gateway from the outside world
   into a Sidecar-based services environment. Working to get this plugin
   pushed upstream.

Overview in Brief
-----------------

Services communicate to each other through an HAproxy instance on each host
that is itself managed and configured by Sidecar. It is inspired by Airbnb's
SmartStack. But, we believe it has a few advantages over SmartStack:

 * Native support for Docker (works without Docker, too!)
 * No dependence on Zookeeper or other centralized services
 * Peer-to-peer, so it works on your laptop or on a large cluster
 * Static binary means it's easy to deploy, and there is no interpreter needed
 * Tiny memory usage (under 20MB) and few execution threads means its very
   light weight

**See it in Action:** We've presented Sidecar at Velocity 2015 and recorded a [YouTube
video](https://www.youtube.com/watch?v=VA43yWVUnMA) demonstrating Sidecar with
[Centurion](https://github.com/newrelic/centurion), deploying services in
Docker containers, and seeing Sidecar discover and health check them. The second
video shows the current state of the UI which is improved since the first video.

[![YouTube Video](views/static/youtube.png)](https://www.youtube.com/watch?v=VA43yWVUnMA)
[![YouTube Video2](views/static/youtube2.png)](https://www.youtube.com/watch?v=5MQujt36hkI)

Complete Overview and Theory
----------------------------

Sidecar is an eventually consistent service discovery platform where hosts learn
about each others' state via a gossip protocol. Hosts exchange messages about
which services they are running and which have gone away. All messages are
timestamped and the latest timestamp always wins. Each host maintains its own
local state and continually merges changes in from others. Messaging is over
UDP except when doing anti-entropy transfers. 

There is an anti-entropy mechanism where full state exchanges take place
between peer nodes on an intermittent basis. This allows for any missed
messages to propagate, and helps keep state consistent across the cluster.

Sidecar hosts join a cluster by having a set of cluster seed hosts passed to them
on the command line at startup. Once in a cluster, the first thing a host does
is merge the state directly from another host. This is a big JSON blob that is
delivered over a TCP session directly between the hosts.

Now the host starts continuously polling its own services and reviewing the
services that it has in its own state, sleeping a couple of seconds in between.
It announces its services as UDP gossip messages every couple of seconds, and
also announces tombstone records for any services which have gone away.
Likewise, when a host leaves the cluster, any peers that were notified send
tombstone records for all of its services. These eventually converge and the
latest records should propagate everywhere. If the host rejoins the cluster, it
will announce new state every few seconds so the services will be picked back
up.

There are lifespans assigned to both tombstone and alive records so that:

1. A service that was not correctly tombstoned will go away in short order
2. We do not continually add to the tombstone state we are carrying

Because the gossip mechanism is UDP and a service going away is a higher
priority message, each tombstone is sent twice initially, followed by
once a second for 10 seconds. This delivers reliable messaging of service
death.

Timestamps are all local to the host that sent them. This is because we can
have clock drift on various machines. But if we always look at the origin timestamp
they will at least be comparable to each other by all hosts in the cluster. The
one exception to this is that if clock drift is more than a second or two, the
alive lifespan may be negatively impacted.

Running it
----------

You can download the latest release from the [GitHub
Releases](https://github.com/Nitro/sidecar/releases) page.

If you'd rather build it yourself, you should install the latest version of
the Go compiler. Sidecar has not been tested with gccgo, only the mainstream
Go compiler.

It's a Go application and the dependencies are all vendored into the `vendor/`
directory so you should be able to build it out of the box.

```bash
$ go build
```

Or you can run it like this:

```bash
$ go run *.go --cluster-ip <boostrap_host>
```

You always need to supply at least one IP address or hostname with the
`--cluster-ip` argument. If are running solo, or are the first member, this can
be your own hostname. You may specify the argument multiple times to have
multiple hosts. It is recommended to use more than one when possible.

### Running in a Container

The easiest way to deploy Sidecar to your Docker fleet is to run it in a
container itself. [Instructions for doing that are provided](docker/README.md).

Nitro Software maintains builds of the [Docker container
image](https://hub.docker.com/r/gonitro/sidecar/) on Docker Hub. Note that
the [README](docker/README.md) describes how to configure this container.


Configuration
-------------

Configuration supports a number of options via a TOML file by default named
`sidecar.toml`. You may also configure the Docker container with some
[environment
variables](https://github.com/Nitro/sidecar/blob/master/docker/s6/services/sidecar.svc/run).
Most of the options are described in more detail in the [example TOML
file](https://github.com/Nitro/sidecar/blob/master/sidecar.example.toml).

If needed, you can specify on the command line that Sidecar should use a
different filename by using the `--config-file` or `-f` option.

Sidecar supports both Docker-based discovery and a discovery mechanism where
you publish services into a JSON file locally. These can then be advertised as
running services just like they would be from a Docker host.

### Discovery

Sidecar currently supports two methods of discovery and these can be set in
the `sidecar.toml` file in the `sidecar` section.

A configuration for both Docker and static discovery looks like this:

```toml
[sidecar]
discovery = [ "docker", "static" ]
```

Zero or more options may be supplied. Note that if nothing is in this section,
Sidecar will only participate in a cluster but will not announce anything.

#### Configuring Docker Discovery

Sidecar currently accepts a single option for Docker-based discovery, the URL
to use to connect to Docker. You really want this to be the local machine.
It uses the same URLs that are supported by the Docker command line tools.
The configuration block for Sidecar looks like this:

```toml
[docker_discovery]
docker_url = "tcp://localhost:2375"
```

Note that it only supports a *single* URL, unlike the Docker CLI tool.

Sidecar can now use the normal Docker environment variables for configuring
Docker discovery. If you remove the `docker_url` setting from the config
entirely, it will fall back to trying to use environment variables to configure
Docker. It uses the standard variables like `DOCKER_HOST`, `TLS_VERIFY`, etc.

##### Labels

A few Docker labels can be used to control the discovery behavior of Sidecar.
Services may be started with one or more `ServicePort_xxx` labels that help
Sidecar to understand ports that are mapped dynamically. This controls the port
on which HAproxy will listen for the service as well. If I have a service where
the container is built with `EXPOSE 80` and I want HAproxy to listen on port
8080 then I will add a Docker label to the service in the form:

```
	ServicePort_80=8080
```

With dynamic port bindings, Docker may then bind that to 32767 but Sidecar will
know which service and port that belongs.

**All containers need to be started with two labels** defining how they are to
be health checked. To health check a service on port 9090 on the local system
with an `HttpGet` check, for example, you would use the following labels:

```
	HealthCheck=HttpGet
	HealthCheckArgs=http://:9090/status
```

The currently available check types are `HttpGet` and `External`. `External`
checks will run the command specified in the `HealthCheckArgs` label (in the
context of a bash shell). An exit status of 0 is considered healthy and
anything else is unhealthy. Nagios checks work very well with this mode of
health checking.

Additionally, it can sometimes be nice to exclude certain containers from
discovery. This is particularly useful if you are running Sidecar in a
container itself. This is accomplished with another Docker label like so:

```
	SidecarDiscover=false
```

By default, HAProxy will run in HTTP mode. The mode can be changed to TCP by setting the following Docker label:

```
ProxyMode=tcp
```

Finally, you sometimes need to pass information in the Docker labels which
is not available to you at the time of container creation. One example of this
is the need to identify the actual Docker-bound port when running the health
check. For this reason, Sidecar allows simple templating in the labels. Here's
an example.

If you have a service that is exposing port 8080 and Docker dynamically assigns
it the port 31445 at runtime, your health check for that port will be impossible
to define ahead of time. But with templating we can say:

```--label HealthCheckArgs="http://{{ host }}:{{ tcp 8080 }}/"```

This will then fill the template fields, at call time, with the current
hostname and the actual port that Docker bound to your container's port 8080.
Querying of UDP ports works as you might expect, by calling `{{ udp 53 }}` for
example.

**Note** that the `tcp` and `udp` method calls in the templates refer only
to ports mapped with `ServicePort` labels. You will need to use the port
number that you expect HAproxy to use.

#### Configuring Static Discovery

Static Discovery requires a configuration block in the `sidecar.toml` that
looks like this:

```toml
[static_discovery]
config_file = "/my_path/static.json"
```

That in turn points to a static discovery file that looks like this:

```json
[
    {
        "Service": {
            "Name": "some_service",
            "Image": "bb6268ff91dc42a51f51db53846f72102ed9ff3f",
            "Ports": [
                {
                    "Type": "tcp",
                    "Port": 10234,
					"ServicePort": 9999
                }
            ],
			"ProxyMode": "http",
        },
        "Check": {
            "Type": "HttpGet",
            "Args": "http://:10234/"
        }
    },
	{
	...
	}
]
```

Here we've defined both the service itself and the health check to use to
validate its status. It supports a single health check per service.  You should
supply something in place of the value for `Image` that is meaningful to you.
Usually this is a version or git commit string. It will show up in the Sidecar
web UI.

Sidecar Events
--------------

Services which need to know about service discovery change events can subscribe
to Sidecar events. Any time a significant change happens, the listener will
receive an update over HTTP from Sidecar. There are three mechanisms by which
a service can subscribe to Sidecar events:

 1. Add the endpoint in the `sidecar.toml` e.g.:
    ```
	[listeners]
	urls = [ "http://localhost:7778/api/update" ]
	```

 2. Add a Docker label to the subscribing service in the form
    `SidecarListener=10005` where 10005 is a port that is mapped to a
    `ServicePort` with a Docker label like `ServicePort_80=10005`. This port will
    then receive all updates on the `/update` endpoint. The subscription will be
    dynamically added and removed when the service starts or stops.

 3. Add the listener export to the `static.json` file exposed by static
    services. The `ListenPort` is a top-level setting for the `Target` and is
	of the form `ListenPort: 10005` inside the `Target` definition.

Monitoring It
-------------

The logging output is pretty (too?) verbose and contains lots of information
about what's going on and what the current state is. Or you can use the web
interface.

Currently the web interface runs on port 7777 on each machine that runs `sidecar`.

The `/services` endpoint is a very textual web interface for humans. The
`/services.json` endpoint is JSON-encoded. The JSON is still pretty-printed so
it's readable by humans.

Sidecar's API
-------------

Other than the UI that lives on the base URL, there is a minimalist API
available for querying Sidecar. It supports the following endpoints:

 * `/services.json`: This returns a big JSON blob sorted and grouped by
   service.
 * `/state.json`: Returns the whole internal state blob in the internal
   representation order (servers -> server -> service -> instances)
 * `/services/<service name>.json`: Returns the same format as the
   `/service.json` endpoint, but only contains data for a single service.
 * `/watch`: Inconsistenly named endpoint that returns JSON blobs on a
   long-poll basis every time the internal state changes. Useful for
   anything that needs to know what the ongoing service status is.

Sidecar can also be configured to post the internal state to HTTP endpoints on
any change event. This is configured with listeners in the TOML file and is
described in the `sidecar.example.toml`.

Contributing
------------

Contributions are more than welcome. Bug reports with specific reproduction
steps are great. If you have a code contribution you'd like to make, open a
pull request with suggested code.

Pull requests should:

 * Clearly state their intent in the title
 * Have a description that explains the need for the changes
 * Include tests!
 * Not break the public API

Ping us to let us know you're working on something interesting by opening a
GitHub Issue on the project.

By contributing to this project you agree that you are granting New Relic a
non-exclusive, non-revokable, no-cost license to use the code, algorithms,
patents, and ideas in that code in our products if we so choose. You also agree
the code is provided as-is and you provide no warranties as to its fitness or
correctness for any purpose

Logo
----
The logo is used with kind permission from [Picture Esk](https://www.flickr.com/photos/22081583@N06/4226337024/).
