# CMOS Prometheus Exporter

This is an **experimental** Prometheus exporter for Couchbase Server.

## Motivation

Couchbase Server 7.0 adds a built-in Prometheus statistics system for [gathering stats](https://docs.couchbase.com/server/current/learn/security/roles.html#external-stats-reader) from a Couchbase Server instance. CMOS Prometheus Exporter is an experiment to determine whether it is possible to create an exporter for Couchbase Server 6.x (and possibly below) exposing the metrics in the same format as 7.x does.

## Installation

Build it from source using [Go](https://golang.org/doc/install). To run it, clone this repository and run `go run ./cmd/cmos-exporter/main.go`.

## Configuration

The exporter is configured using a `cmos-exporter.yml` file. The configuration options are enumerated in [`pkg/config/config.go`](pkg/config/config.go).

```yaml
couchbase_host: 10.112.208.101 # IP address or hostname of the Couchbase Server instance
couchbase_username: Administrator # Username
couchbase_password: password # Password
bind: 0.0.0.0:9091 # host:port to bind the HTTP server on
fake_collections: true # whether to add `scope` and `collection` labels (with a value of `_default`) to all metrics that have them in 7.x
```
