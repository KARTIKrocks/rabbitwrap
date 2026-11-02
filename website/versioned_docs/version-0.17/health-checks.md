---
id: health-checks
title: Health Checks
description: Checking connection health for readiness and liveness probes.
---

# Health Checks

```go
if conn.IsHealthy() {
    // Connection is open and responsive
}

if conn.IsClosed() {
    // Connection has been closed
}
```

`IsHealthy` reflects the connection only — a channel-level exception on a
publisher or consumer is caught and repaired independently (see
[Connection → Channel Recovery](./connection.md#channel-recovery)) and does
not flip it to unhealthy.
