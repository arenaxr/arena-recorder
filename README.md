# ARENA 3D Replay Recorder

The `arena-recorder` is a **Go-based microservice** that handles the ingestion, buffering, and storage of MQTT messages for the ARENA 3D Replay ecosystem.

## Overview
This service listens for REST API calls to start and stop recordings. When a recording is started, it dynamically subscribes to the ARENA MQTT broker for a specific namespace and scene (`realm/s/<namespace>/<scene>/#`), captures the live scene state using the `arena-persist` REST database, and begins streaming incoming MQTT JSON messages into local chunked files (`.jsonl`).

> [!NOTE]
> For in-depth architecture diagrams, source code mappings, and dataflow specifications, please read [REQUIREMENTS.md](REQUIREMENTS.md).

## Features
- **JWT Authentication:** Strict authorization enforcement using ARENA `mqtt_token` cookies. Both recording controls and resulting session streaming natively enforce proper `publ` and `subl` topic ACLs.
- **Concurrent Ingestion:** Leverages Go goroutines and channels to handle high-throughput MQTT packet buffering simultaneously across heavy multi-user scenes without blocking the main event loop.
- **Direct File Streaming:** Writes directly to disk using `bufio` to accommodate massive continuous 3D scene data without bloating container memory or risking Docker OOM kills.
- **REST Playback API:** Exposes Nginx-proxied endpoints to query and securely stream `.jsonl` payload chunks safely for the local 3D Replay browser viewer without proxying historical MQTT data directly.

## Configuration
The `arena-recorder` requires its own isolated JWT service token and MQTT connection config to verify permissions and connect to the core message broker.
This is automatically setup by `arena-services-docker/init-config.sh` which provisions `conf/recorder-config.json` and mounts it natively into the container stack at `/app/config.json`.
Local storage payloads are dropped into a persistent `/recording-store` volume.

## API Endpoints
All traffic is routed natively through the main Nginx router block `/recorder/`.

- `POST /recorder/start`: Begin recording a scene segment. Requires JWT authentication with write (`publ`) permissions over the namespace.
- `POST /recorder/stop`: Terminate an active recording session manually.
- `GET /recorder/list`: List all available recordings mapped against the client's readable scopes.
- `GET /recorder/status`: Return a strict boolean on whether a scene is currently recording.
- `GET /recorder/files/{filename}`: Securely stream the raw recording `.jsonl` payload segment over HTTP directly to the client playback engine.

## Development & Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md) and [REQUIREMENTS.md](REQUIREMENTS.md) for deeper architecture specifics on how to extend and maintain this service.
