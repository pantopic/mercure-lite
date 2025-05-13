# Mercure Lite

A partially implementation of the Mercure protocol.

[![Go Reference](https://godoc.org/github.com/pantopic/mercure-lite?status.svg)](https://godoc.org/github.com/pantopic/mercure-lite)
[![Go Report Card](https://goreportcard.com/badge/github.com/pantopic/mercure-lite?4)](https://goreportcard.com/report/github.com/pantopic/mercure-lite)
[![Go Coverage](https://github.com/pantopic/mercure-lite/wiki/coverage.svg)](https://raw.githack.com/wiki/pantopic/mercure-lite/coverage.html)

## Why Partial?

Mercure has a number of features that not everybody needs. The ability to express topic selectors as uri templates makes the protocol more flexible but also degrades performance. Some users of Mercure are paying a performance penalty for a feature they aren't using.

__Mercure Lite__ does not implement:

- Integrated TLS Termination (Caddy)
- URI Template Topic Selectors
- Storage

## Roadmap

- `v0.0.x` - Alpha
  - [ ] Add support for more (all) JWT key algorithms
  - [ ] Test more failure scenarios (ie. malformed keys, tokens, etc)
  - [ ] Implement `"*"` topic
- `v0.x.x` - Beta
  - [ ] Add storage capabilities
  - [ ] Add support for `last-event-id`
- `v1.x.x` - General Availability
  - [ ] Distributed (multi-node) capabilities

## License

This is a ground-up implementation of the Mercure protocol licensed under Apache 2.0
