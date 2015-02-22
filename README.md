Bosun
=====

Bosun is a service discovery platform for Docker that uses a gossip protocol
for all communication between hosts. It is intended to run on Docker hosts, but
any host can join the cluster. Only Docker hosts will export services to the
cluster.

Running it
----------

It's a Go application so just build it with:

```bash
$ go build
```

Or you can run it like this:

```bash
$ go run *.go -cluster-ip <boostrap_host>
```

You always need to supply at least one IP address or hostname with the
`--cluster-ip` argument. If are running solo, or are the first member, this can
be your own hostname. You may specify the argument multiple times to have
multiple hosts. It is recommended to use more than one when possible.

Monitoring It
-------------

The logging output is pretty verbose and contains lots of information about
what's going on and what the current state is. Or you can use the web
interface.

Currently the web interface runs on port 7777 on each machine that runs `bosun`.

The `/services` endpoint is a very textual web interface for humans. The
`/services.json` endpoint is JSON-encoded. The JSON is still pretty-printed so
it's readable by humans.

Theory
------

Bosun is an eventually consistent service discovery platform where hosts learn
about each others' state via a gossip protocol. Hosts exchange messages about
which services they are running and which have gone away. All messages are
timestamped and the latest timestamp always wins. Each host maintains its own
local state and continually merges changes in from others. Messaging is over
UDP except when doing anti-entropy transfers.

There is an anti-entropy mechanism where full state exchanges take place
between peer nodes on an intermittent basis. This allows for any missed
messages to propagate, and helps keep state consistent across the cluster.

Bosun hosts join a cluster by having a set of cluster seed hosts passed to them
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
