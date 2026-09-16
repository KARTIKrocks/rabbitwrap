---
id: development
title: Development
description: Running unit and integration tests, thread-safety guarantees, and contributing.
---

# Development

```bash
# Run unit tests
make test

# Run go vet + golangci-lint (incl. staticcheck) + tests
make ci

# Run integration tests (requires Docker)
make test-integration

# Start RabbitMQ locally
make docker-up
```

## Thread Safety

- `Connection` — safe for concurrent use
- `Publisher` — safe for concurrent use
- `Consumer` — use one goroutine per consumer; create multiple consumers for
  parallel processing

## Contributing

See [CONTRIBUTING.md](https://github.com/KARTIKrocks/rabbitwrap/blob/main/CONTRIBUTING.md)
on the main branch — that's where the Go module itself lives; this
documentation site lives on the `website` branch (see this site's
[README](https://github.com/KARTIKrocks/rabbitwrap/blob/website/README.md)).

## License

[MIT](https://github.com/KARTIKrocks/rabbitwrap/blob/main/LICENSE)
