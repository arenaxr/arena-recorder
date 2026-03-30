# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - $(date +'%Y-%m-%d')
### Added
- Initial release of the `arena-recorder` microservice.
- REST API points for `/recorder/start`, `/recorder/stop`, `/recorder/list`, and `/recorder/files`.
- MQTT topic subscription for live scene packet capture.
- `arena-persist` DB bootstrapping to capture $t=0$ scene state snapshots.
- JWT cookie `mqtt_token` authentication middleware to enforce standard ARENA ACL checking.
- Isolated docker container architecture connected to `recording-store` volume.
