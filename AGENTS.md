# Agent Guide

Orientation for agents (and humans) working in this repo. Detailed docs live in the files below — this file is just the index.

## Start here
- [README.md](README.md) — what arena-recorder is: a Go-based microservice for ingesting, buffering, and storing MQTT messages for 3D Replay.
- [REQUIREMENTS.md](REQUIREMENTS.md) — machine- and human-readable reference for features, architecture, dataflow, and source layout.

## Conventions & development rules
- [CONTRIBUTING.md](CONTRIBUTING.md) — mandatory rules for all contributors, **including agents**: MQTT topic construction, development rules.

## Tests
- `go test ./...` — the full suite (38 tests); needs no broker, `arena-persist`, or config file.
- [auth/jwt_test.go](auth/jwt_test.go) — token validation and topic ACLs. [mqtt/recorder_test.go](mqtt/recorder_test.go) — deep merge, state tracking, keyframing, index repair. `api/server.go` is untested.
- [CONTRIBUTING.md](CONTRIBUTING.md) — the `gofmt`/`build`/`vet`/`test` checks CI gates, and single-package or single-test invocations.

## Release history
- [CHANGELOG.md](CHANGELOG.md) — generated release history (release-please; Conventional Commits).
