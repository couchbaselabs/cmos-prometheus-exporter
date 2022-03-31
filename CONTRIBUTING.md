# Contributing to cmos-prometheus-exporter

Hi, and thanks for taking the time to contribute to cmos-prometheus-exporter!
This guide will hopefully cover everything you need to know to contribute to it.
If you notice anything missing or incorrect, please file an issue, and consider contributing a fix yourself - you might save other people countless hours!

## Tracking Issues

// TBD

## Building and Running

cmos-prometheus-exporter is written in Go and requires at least Go 1.18 to build.
You can download Go from the [official website](https://go.dev) or using your OS's package manager ([Homebrew](https://formulae.brew.sh/formula/go#default) on macOS).

To run it, simply run `go run ./cmd/cmos-exporter`. This will automatically install any dependencies if necessary. There is a sample `cmos-exporter.yml` configuration file that you may need to fill in with the details of a running Couchbase Server cluster

Note that some features (namely XDCR metrics) are only available if the exporter is running on the same logical computer as Couchbase Server (in other words, it can access it on `127.0.0.1`).

If you use Docker, there is also a Docker Compose file in [tools/testing](tools/testing) that sets up two single-node clusters with all the features (except Analytics) configured. Note that you will need to run it using `docker compose up --build` to rebuild the Exporter image if you make any changes.

## Code Style

In general, we follow the code style enforced by `go fmt` and [`gofumpt`](https://github.com/mvdan/gofumpt).

To check that your code follows the standards, install [golangci-lint](https://golangci-lint.run), then run `golangci-lint run`.
This is also checked automatically when you submit a pull request.

## Contributing Code Changes

We use GitHub pull requests to contribute code, following the [GitHub Flow](https://githubflow.github.io/) with one key difference - we always rebase commits onto `main` rather than merging (don't worry if you're not sure what that means!).

In practical terms, you will need to fork the repository (unless you are a member of the `couchbaselabs` organisation on GitHub), create a branch, make a commit with your changes, and create a pull request. [GitHub's documentation](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/getting-started/about-collaborative-development-models) explains each step very well.

### Commit Message Conventions

By convention, our commit messages look like this:

```
package: what this commit does

A longer description of what this commit does. In some cases, the
commit message may be longer than the code change itself - this context
can often be invaluable.

Try to keep your commit message to no more than 72 characters across,
as this ensures it doesn't get wrapped in terminals.
```

The `package` on the first line should be the name of the Go package that the commit most touches (e.g. `metrics/memcached` or `cmd/cmos-exporter`), with the following exceptions:
* The `pkg` prefix can be omitted (so a commit that changes `pkg/couchbase` would have a commit message prefix of `couchbase`)
* Changes to [pkg/metrics/defaultMetricSet.json](pkg/metrics/defaultMetricSet.json) should use the package of the metrics that the commit changes (for example, a change to the query metrics in `defaultMetricSet.json` would use `metrics/n1ql` as the package)
* Changes to the GitHub Actions configuration in [.github/workflows](.github/workflows) should use `workflows` as the package
* Changes that touch lots of files across the entire codebase can use `all` as a package

The remainder of the first line should complete the following sentence: `This commit changes cmos-prometheus-exporter to ___.` In other words, it should be a complete sentence without a capital letter at the start or punctuation at the end, written in the imperative case (so "fix bug" rather than "fixed" or "fixes").

Here is an example commit message:

```
cmd/cmos-exporter: log version at Info level

Previously we would log the version as part of the same line that logs
the configuration, which was at debug level, and thus disabled by
default. Instead, log it at info level which is enabled by default.
```
