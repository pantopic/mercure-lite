# Mercure Lite

A partial implementation of [The Mercure Protocol](https://www.ietf.org/archive/id/draft-dunglas-mercure-07.html).

[![Go Reference](https://godoc.org/github.com/pantopic/mercure-lite?status.svg)](https://godoc.org/github.com/pantopic/mercure-lite)
[![License](https://img.shields.io/badge/License-Apache_2.0-dd6600.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/pantopic/mercure-lite?4)](https://goreportcard.com/report/github.com/pantopic/mercure-lite)
[![Go Coverage](https://github.com/pantopic/mercure-lite/wiki/coverage.svg)](https://raw.githack.com/wiki/pantopic/mercure-lite/coverage.html)

The mercure protocol contains a number of features that many users don't need. The ability to express topic selectors as uri templates makes the protocol more flexible but also presents performance and scalability challenges.

This project implements 80% of the Mercure protocol in 20% as many lines of code as the canonical implementation.

## Performance

Mercure Lite performance is similar to Mercure for local transport at low concurrency.
```bash
# Mercure
> make parity
2025/05/18 10:04:29 Starting 256 subscribers
2025/05/18 10:04:29 Starting 16 publishers
2025/05/18 10:04:30 Sending 10000 messages
2025/05/18 10:04:32 10000 sent, 10000 received in 1.488329306s (6718.94 msgs/sec)

# Mercure Lite
> make loadtest
2025/05/18 11:10:44 Starting 256 subscribers
2025/05/18 11:10:44 Starting 16 publishers
2025/05/18 11:10:45 Sending 10000 messages
2025/05/18 11:10:47 10000 sent, 10000 received in 1.206347937s (8289.53 msgs/sec)
```

Mercure Lite can significantly outperform Mercure at high concurrency.

This test uses Caddy running on a single AWS EC2 `c7i.xlarge` (4 cpu cores).
```bash
# Mercure
$ go run main.go -s 20000 -c 256 -n 50000 -target https://test.hky.me
2025/05/18 22:37:03 Starting 20000 subscribers
2025/05/18 22:37:09 Starting 256 publishers
2025/05/18 22:37:10 Sending 50000 messages
2025/05/18 22:40:07 50000 sent, 50000 received in 2m57.127s (282.28 msgs/sec)

# Mercure Lite
$ go run main.go -s 20000 -c 256 -n 50000 -target https://test.hky.me
2025/05/18 22:40:56 Starting 20000 subscribers
2025/05/18 22:41:02 Starting 256 publishers
2025/05/18 22:41:03 Sending 50000 messages
2025/05/18 22:41:24 50000 sent, 50000 received in 21.572s (2317.80 msgs/sec)
```

See [cmd/loadtest](cmd/loadtest/main.go) for specifics

## Adoption

__Mercure Lite__ might be right for you if you do _not_ need:

- Integrated TLS Termination
- URI Template topic selectors
- Subscription to the reserved `"*"` topic

## Deployment

Mercure Lite runs as its own process (port 8001 by default).

You can put it behind Caddy for TLS termination using the `reverse_proxy` Caddyfile directive:

```ini
# Caddyfile
test.hky.me {
        reverse_proxy {
                to localhost:8001
        }
}
```

## Roadmap

- `v0.0.x` - Alpha
  - [X] Add support for more (all) JWT key algorithms
  - [X] Close connections made with tokens that expire while connection is open
  - [X] Test more failure scenarios (ie. malformed keys, tokens, etc)
- `v0.x.x` - Beta
  - [ ] Add storage capabilities
  - [ ] Add support for `last-event-id`
  - [ ] Distributed (multi-node) capabilities
- `v1.x.x` - General Availability

## License

This is a ground-up implementation of a Mercure protocol hub licensed under Apache 2.0
