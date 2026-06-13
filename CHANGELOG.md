# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-03-08

### Added
- Connection management with automatic reconnection and exponential backoff
- Publisher with confirm mode support
- Consumer with prefetch and manual acknowledgment
- Fluent message builder API
- Batch publishing support
- Queue and exchange declaration helpers
- Dead letter queue and quorum queue support
- Consumer middleware (logging, recovery, retry)
- TLS support
- Health checks
- Structured logging via pluggable Logger interface
