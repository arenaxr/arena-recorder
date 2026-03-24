# ARENA 3D Replay Recorder

The `arena-recorder` is a Go-based microservice that handles the ingestion, buffering, and storage of MQTT messages for the ARENA 3D Replay system.

## Overview
This service listens for REST API calls to start and stop recordings. When a recording is started, it dynamically subscribes to the ARENA MQTT broker for a specific namespace and scene (`realm/s/<namespace>/<scene>/#`), captures the live scene state using the `arena-persist` REST DB, and begins streaming incoming MQTT JSON messages into local chunked files (`.jsonl`).

## Features
- **JWT Authentication:** Uses the exact same ARENA JWT cookies (`mqtt_token`) and ACL validation logic as the live client for secured recordings.
- **Concurrent Ingestion:** Leverages Go goroutines and channels to handle high-throughput MQTT packet buffering without blocking the main event loop.
- **Direct File Streaming:** Writes directly to disk using `bufio` to accommodate large 3D scene data without blowing up container memory.
- **REST Playback API:** Exposes endpoints to query and download `.jsonl` payload chunks safely for the 3D Replay viewer.

## Configuration
The `arena-recorder` requires its own isolated JWT service token and MQTT connection config. This is automatically setup by `arena-services-docker/init-config.sh` which provisions `conf/recorder-config.json` and mounts it into the container.

## API Endpoints
- `POST /recorder/start`: Begin recording a scene segment. Requires JWT auth and write permissions.
- `POST /recorder/stop`: Terminate an active recording session.
- `GET /recorder/list`: List all available recordings for a namespace/scene.
- `GET /recorder/files/:filename`: Stream the recording `.jsonl` payload.

## Development
See `CONTRIBUTING.md` and `REQUIREMENTS.md` for details on how to extend and maintain this service.
