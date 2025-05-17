# Mercure Lite

A partial implementation of the [Mercure protocol](https://www.ietf.org/archive/id/draft-dunglas-mercure-07.html).

[![Go Reference](https://godoc.org/github.com/pantopic/mercure-lite?status.svg)](https://godoc.org/github.com/pantopic/mercure-lite)
[![License](https://img.shields.io/badge/License-Apache_2.0-orange.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/pantopic/mercure-lite?4)](https://goreportcard.com/report/github.com/pantopic/mercure-lite)
[![Go Coverage](https://github.com/pantopic/mercure-lite/wiki/coverage.svg)](https://raw.githack.com/wiki/pantopic/mercure-lite/coverage.html)

Mercure has a number of features that not everybody needs. The ability to express topic selectors as uri templates makes the protocol more flexible but also presents performance and scalability challenges.

This project implements 80% of the Mercure protocol in 20% as many lines of code as the canonical implementation. It is equally stable and secure trading fewer features for a 30x improvement in throughput.

__Mercure Lite__ does not implement:

- Integrated TLS Termination (Caddy)
- URI Template Topic Selectors
- The reserved `"*"` topic for subscriptions

## Performance

Mercure Lite exhibits up to 30x higher throughput than Mercure in load tests.
```
> make dev
> make loadtest
2025/05/17 11:35:37 Starting 256 subscribers
2025/05/17 11:35:37 Starting 16 publishers
2025/05/17 11:35:37 Sending 10000 messages
2025/05/17 11:35:38 10000 sent, 10000 received in 1.395261s

> make parity-target
> make parity
2025/05/17 11:36:03 Starting 256 subscribers
2025/05/17 11:36:03 Starting 16 publishers
2025/05/17 11:36:04 Sending 10000 messages
2025/05/17 11:36:48 10000 sent, 10000 received in 42.476316948s
```

See [cmd/loadtest](cmd/loadtest/main.go) for specifics

## Roadmap

- `v0.0.x` - Alpha
  - [X] Add support for more (all) JWT key algorithms
  - [ ] Test more failure scenarios (ie. malformed keys, tokens, etc)
- `v0.x.x` - Beta
  - [ ] Add storage capabilities
  - [ ] Add support for `last-event-id`
  - [ ] Distributed (multi-node) capabilities
- `v1.x.x` - General Availability

## License

This is a ground-up implementation of a Mercure protocol hub licensed under Apache 2.0
