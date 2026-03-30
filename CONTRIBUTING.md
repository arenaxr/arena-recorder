# Contributing to arena-recorder

We welcome contributions! Please follow these guidelines when contributing to the `arena-recorder` microservice.

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
