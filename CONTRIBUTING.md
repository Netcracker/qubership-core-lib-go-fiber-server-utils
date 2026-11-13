# Contribution Guide

We'd love to accept patches and contributions to this project.
Please, follow these guidelines to make the contribution process easy and effective for everyone involved.

## Contributor License Agreement

You must sign the [Contributor License Agreement](https://pages.netcracker.com/cla-main.html) in order to contribute.

## Code of Conduct

Please make sure to read and follow the [Code of Conduct](CODE-OF-CONDUCT.md).

## Tests

Tests use the Go `testing` engine with testify `assert` and `require`.
Run `go test ./...` for the package suites. CI also runs `go build ./...`, `go vet ./...`,
and `go test -race -p 1 -count=1 -timeout 5m ./...`. The race run serializes packages because
the existing Zipkin fixtures use a shared local port. Listener tests exercise HTTP and TLS
on IPv4 and skip IPv6 cases only when the runner has no IPv6 loopback.
