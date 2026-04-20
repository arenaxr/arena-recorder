# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.0.0 (2026-04-20)


### Features

* add shallow delta diffing on updates ([5c3cc36](https://github.com/arenaxr/arena-recorder/commit/5c3cc3665a45cf202f16a0c3ba8b237f4ed061fd))
* **recorder:** add inline keyframing for seek ([e1b1cc9](https://github.com/arenaxr/arena-recorder/commit/e1b1cc928cff594f95101fea1da1efdebec9c757))
* **repair:** add CLI tool to rebuild keyframe idx ([b8dd03a](https://github.com/arenaxr/arena-recorder/commit/b8dd03a33da745dfa539fe501c598e0dd3e5d0e9))
* **replay:** add auth checks replay/record ([a27ea3e](https://github.com/arenaxr/arena-recorder/commit/a27ea3e0bc96dfeccfb4db1789219ddcfab824fe))
* **replay:** add auth verify, .jsonl record and replay ([68ecdf9](https://github.com/arenaxr/arena-recorder/commit/68ecdf98a69f37e04ba59d1894eed7be1a0e610d))
* **replay:** add persitance scene banner when recording ([6bd98a9](https://github.com/arenaxr/arena-recorder/commit/6bd98a9d20f2d435ce2dde8d1c6c30abed856539))
* **replay:** stand up docker arena-recorder microservice ([5468a16](https://github.com/arenaxr/arena-recorder/commit/5468a16060553a1037caed197d5206238aa11f45))


### Bug Fixes

* add buffered writes, MQTT reconnection, and fix chat publish ordering ([1b9d8c3](https://github.com/arenaxr/arena-recorder/commit/1b9d8c3e4f84d2d37fbf9fe84ed46972f1c8737d))
* add status 200 for preflight ([9218320](https://github.com/arenaxr/arena-recorder/commit/9218320e31f1d7593df31894028ba45e68422334))
* cache RSA public key and remove unused JWKSecret placeholder ([421e249](https://github.com/arenaxr/arena-recorder/commit/421e2492ab2dc46be18d663986e18ac83173dbf3))
* enforce ACL on file downloads and harden path traversal ([1dcafc9](https://github.com/arenaxr/arena-recorder/commit/1dcafc92578e61b5d7686e8f93a3d51be23e0504))
* pr review issues ([b6cdc8a](https://github.com/arenaxr/arena-recorder/commit/b6cdc8a4711248936d1b724e6cb64d358bf591a9))
* prevent FormatTopic from mutating caller's map ([a2d24d1](https://github.com/arenaxr/arena-recorder/commit/a2d24d189a75d6da0ab4bd62b489f96ba26fb270))
* **replay:** ensure consistant timestamps from persist load ([40a360d](https://github.com/arenaxr/arena-recorder/commit/40a360d07073a8aeaca0c1b298513b76a327af8f))
* **replay:** file parse fix, open rights ([04ceebd](https://github.com/arenaxr/arena-recorder/commit/04ceebddbf59a8d07b70ce985b019a617156eeeb))
* review changes ([56deceb](https://github.com/arenaxr/arena-recorder/commit/56deceb8fd0080c927a3c5e6fe086e9d3d2cb0b5))
* upgrade golang to 1.26 ([f6fa130](https://github.com/arenaxr/arena-recorder/commit/f6fa130bf0251a78f7151699bcd75ad8cb3517bd))

## [0.1.0] - $(date +'%Y-%m-%d')
### Added
- Initial release of the `arena-recorder` microservice.
- REST API points for `/recorder/start`, `/recorder/stop`, `/recorder/list`, and `/recorder/files`.
- MQTT topic subscription for live scene packet capture.
- `arena-persist` DB bootstrapping to capture $t=0$ scene state snapshots.
- JWT cookie `mqtt_token` authentication middleware to enforce standard ARENA ACL checking.
- Isolated docker container architecture connected to `recording-store` volume.
