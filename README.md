# Mercure Lite

A partial implementation of the Mercure protocol.

[![Go Reference](https://godoc.org/github.com/pantopic/mercure-lite?status.svg)](https://godoc.org/github.com/pantopic/mercure-lite)
[![Go Report Card](https://goreportcard.com/badge/github.com/pantopic/mercure-lite?4)](https://goreportcard.com/report/github.com/pantopic/mercure-lite)
[![Go Coverage](https://github.com/pantopic/mercure-lite/wiki/coverage.svg)](https://raw.githack.com/wiki/pantopic/mercure-lite/coverage.html)

Mercure has a number of features that not everybody needs. The ability to express topic selectors as uri templates makes the protocol more flexible but also presents performance and scalability challenges. Some users of Mercure are paying a performance penalty for features they don't need. 

This project aims to implement 80% of the Mercure protocol with 20% as many lines of code compared to other implementations. It should be equally stable and secure having better performance and fewer features.

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
