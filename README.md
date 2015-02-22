Bosun
=====

Bosun is a service discovery platform for Docker that uses a gossip protocol
for all communication between hsots. It is intended to run on Docker hosts, but
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
$ go run bosun.go cli.go docker.go http.go services_delegate.go -cluster-ip <boostrap_host>
```

You always need to supply at least one IP address or hostname with the
`-cluster-ip` address. If are running solo, or are the first member, this can
be your own hostname.

Monitoring It
-------------

The logging output is pretty verbose and contains lots of information about
what's going on and what the current state is. Or you can use the web
interface.

Currently the web interface runs on port 7777 on each machine that runs `bosun`.

The `/services` endpoint is a very textual web interface for humans. The
`/services.json` endpoint is JSON-encoded. The JSON is still pretty-printed so
it's readable by humans.
