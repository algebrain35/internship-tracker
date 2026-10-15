# internship-tracker

A Go application that crawls Canadian tech internship postings from
Greenhouse, Lever, and Workday, persists them in SQLite, and streams
them live to a browser frontend over WebSockets.

## Architecture

```
cmd/crawler  →  internal/crawler  →  internal/store (SQLite, WAL mode)
                                              ↑
cmd/server   →  internal/hub      ←  SQLite poller (rowid cursor)
                     ↓
              WebSocket clients (web/index.html)
```

Two separate processes share a single `jobs.db` SQLite file.
WAL journal mode lets the crawler write while the server reads
without blocking.

## Key Go concepts demonstrated

- Goroutines and buffered channels (fan-out / fan-in pipeline)
- `sync.Map` for lock-free deduplication across goroutines
- Custom errgroup (fail-fast context cancellation)
- Channel nil-pattern for safe fan-in without deadlock
- `net/http` connection hijacking for zero-dependency WebSockets
- SQLite rowid as a monotonic cursor for incremental polling

## Build

Requires `gcc` for CGo (SQLite driver).

```sh
make build          # produces bin/crawler and bin/server
make run            # builds, runs crawler in background, starts server
```

Or manually:

```sh
go build -mod=mod ./cmd/crawler
go build -mod=mod ./cmd/server
./crawler           # crawls and populates jobs.db
./server            # serves http://localhost:8080
```

## Sources

Internship postings are discovered dynamically by parsing crowdsourced
Canadian tech internship lists on GitHub, extracting company slugs from
real apply URLs. Falls back to hardcoded lists if GitHub is unreachable.
