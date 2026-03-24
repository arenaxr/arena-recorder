# Requirements for arena-recorder

This document outlines the expectations and requirements for code maintainers (both human and AI assistants).

## Architecture Constraints
- **Separation of Concerns:** The recorder MUST NOT mutate any live database records in `arena-persist`. It strictly consumes `arena-persist` via standard REST/GraphQL queries to bootstrap $t=0$ keyframes.
- **File System Usage:** Writing `.jsonl` payloads MUST be buffered (e.g. using `bufio`). Avoid accumulating large 3D scene mutations entirely in memory to prevent Docker container OOM kills.
- **ACL Integrity:** Maintain the JWT middleware located in `auth/jwt.go`. The recorder is exposed externally through the Nginx proxy; all `/recorder/start` calls must strictly validate the cookie `mqtt_token` and verify the user has publish/manage rights for the requested scene namespace.

## AI Assistant Prompt Instructions
When interacting with AI coding assistants regarding this service, enforce these rules:
1. **Never use standard `localStorage` or `sessionStorage` logic** for authentication — this service relies entirely on HTTP-only JWT cookies set by the Portal.
2. **Goroutine Leakage:** Always verify that every started goroutine for recording has a deterministic exit condition (e.g. context cancellation, timeout, or explicit `/recorder/stop` signal).
3. **Dependencies:** Attempt to stick to the Go standard library (`net/http`, `encoding/json`, `bufio`, `crypto/rsa`) where possible. Only augment `go.mod` if strictly necessary to avoid supply chain bloat.

## Versioning & Releases
Use [Release Please](https://github.com/googleapis/release-please) for automated versioning. Ensure all commits merged to the `master`/`main` branch follow Conventional Commits formatting.
