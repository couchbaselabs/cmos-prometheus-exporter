# Yet Another Couchbase Prometheus Exporter (Yacpe)

*(Note: this is an experimental tool. I make no guarantees about its compatibility or usefulness (or even it working, really).*

Yacpe is an **experimental** Prometheus exporter for Couchbase Server.

## Motivation

Couchbase Server 7.0 adds a built-in Prometheus statistics system for [gathering stats](https://docs.couchbase.com/server/current/learn/security/roles.html#external-stats-reader) from a Couchbase Server instance. Yacpe is an experiment to determine whether it is possible to create an exporter for Couchbase Server 6.x (and possibly below) exposing the metrics in the same format as 7.x does.

## Installation

If you use Docker, yacpe is published as an image at `ghcr.io/markspolakovs/yacpe`. First create a config file (like in the below section), then run:

```sh
$ docker run -v $(pwd)/yacpe.yaml:/yacpe.yaml -p 9091:9091 ghcr.io/markspolakovs/yacpe
```

Currently no binaries are provided so you will need to build it from source using [Go](https://golang.org/doc/install). To run Yacpe, clone this repository and run `go run ./cmd/yacpe/main.go`.

## Configuration

Yacpe is configured using a `yacpe.yaml` file. The configuration options are enumerated in [`pkg/config/config.go`](https://github.com/markspolakovs/yacpe/blob/master/pkg/config/config.go).

```yaml
couchbase_host: 10.112.208.101 # IP address or hostname of the Couchbase Server instance
couchbase_username: Administrator # Username
couchbase_password: password # Password
bind: 0.0.0.0:9091 # host:port to bind the Yacpe HTTP server on
fake_collections: true # whether to add `scope` and `collection` labels (with a value of `_default`) to all metrics that have them in 7.x
```
