# Contributing to ARENA Recorder

The general Contribution Guide for all ARENA projects can be found [here](https://docs.arenaxr.org/content/contributing.html).

## Development Rules

### 1. MQTT Topics — Always Use the `TOPICS` Constructor

**Never hardcode MQTT topic strings.** All topic paths must be constructed using the local `TOPICS` string constructor for ease of future topics modulation. This enables future topic format refactoring without scattered string updates.

### 2. Dependencies — Pin All Versions

**All dependencies must use exact, pegged versions** (no `^`, `~`, or `*` ranges). This prevents version drift across environments and ensures reproducible builds for security. Use standard `go mod` commands if adding new dependencies, keeping the surface area as lean as possible.

## Local Development

To develop the `arena-recorder` locally:
1. Run `init-config.sh` in the parent `arena-services-docker` directory to generate the required `.env` secrets and the isolated `conf/recorder-config.json`.
2. Start the local stack using `docker-compose -f docker-compose.localdev.yaml up -d arena-recorder`
3. The recorder source is mounted via the Dockerfile build step. For hot-reloading (if configured) or testing, you can modify the `.go` source files and rebuild the container.

## Code Style
- Follow standard Go formatting guidelines (`gofmt`).
- Use contextual logging where appropriate to track goroutine lifecycles and MQTT buffering states.
- Ensure all HTTP handlers return standard JSON error payloads on failure.

The `arena-recorder` uses [Release Please](https://github.com/googleapis/release-please) to automate CHANGELOG generation and semantic versioning. Your PR titles *must* follow Conventional Commit standards (e.g., `feat:`, `fix:`, `chore:`).

> [!CAUTION]
> **Never use `BREAKING CHANGE` in commit/PR bodies or the `!` suffix on commit/PR types (e.g., `feat!:`, `fix!:`).** These tokens cause release-please to automatically bump the major version. Major version increments are reserved for the maintainer's explicit decision — contributors and agents do not decide what constitutes a breaking change for semver purposes.

## Architecture Constraints & Code Maintenance Instructions

- **Separation of Concerns:** The recorder MUST NOT mutate any live database records in `arena-persist`. It strictly consumes `arena-persist` via standard REST/GraphQL queries to bootstrap $t=0$ keyframes.
- **File System Usage:** Writing `.jsonl` payloads MUST be buffered (e.g. using `bufio`). Avoid accumulating large 3D scene mutations entirely in memory to prevent Docker container OOM kills.
- **ACL Integrity:** Maintain the JWT middleware located in `auth/jwt.go`. The recorder is exposed externally through the Nginx proxy; all `/recorder/start` calls must strictly validate the cookie `mqtt_token` and verify the user has publish/manage rights for the requested scene namespace.
- **Goroutine Leakage:** Always verify that every started goroutine for recording has a deterministic exit condition (e.g. context cancellation, timeout, or explicit `/recorder/stop` signal).
- **Dependencies:** Attempt to stick to the Go standard library (`net/http`, `encoding/json`, `bufio`, `crypto/rsa`) where possible. Only augment `go.mod` if strictly necessary to avoid supply chain bloat.


## CI & Dependency Management Conventions
- **GitHub Actions Tag SHA Pinning**: All GitHub Action references in `.github/workflows/` MUST be pinned to the exact commit SHA of the official release tag (e.g., `uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4.4.0`).
- **Inline Version Comments**: The inline comment next to the SHA MUST specify the exact tag version used. This enables Dependabot to recognize the release version, generate human-readable SemVer PR titles (`from X.Y.Z to A.B.C`), and automatically update version comments during upgrades.