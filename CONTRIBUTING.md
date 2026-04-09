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

## Pull Requests
1. Fork the repository and create your feature branch: `git checkout -b feature/my-new-feature`
2. Commit your changes following [Conventional Commits](https://www.conventionalcommits.org/): `git commit -m "feat: Add new awesome feature"`
3. Push to the branch: `git push origin feature/my-new-feature`
4. Submit a pull request.

The `arena-recorder` uses [Release Please](https://github.com/googleapis/release-please) to automate CHANGELOG generation and semantic versioning. Your PR titles *must* follow Conventional Commit standards (e.g., `feat:`, `fix:`, `chore:`).
