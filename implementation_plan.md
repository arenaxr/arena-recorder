# ARENA 3D Replay Refinements

## Proposed Changes

### 1. Topic Structure Refactoring (`arena-recorder`)
- **Action**: Create `topics.go` inside `arena-recorder/mqtt/` to mirror the string literal substitution graph found in `arena-persist/topics.js`.
- **Details**: `topics.go` will define a set of constants or variables for all valid ARENA scene topics (`SCENE_PRESENCE`, `SCENE_OBJECTS`, `SCENE_RENDER`, etc.) using `fmt.Sprintf` or string replacements to mimic the JavaScript template literals.
- **Affected Files**: 
  - `arena-recorder/mqtt/topics.go` [NEW]
  - `arena-recorder/mqtt/recorder.go` [MODIFY] (will use new topic definitions)
  - `arena-recorder/auth/jwt.go` [MODIFY] (if topic strings are hardcoded there)

### 2. Recording Authorization Isolation
- **Action**: Enforce that only users with the `publ` right to the scene's objects can start or stop recordings, and only show recording UI to those users.
- **Details**:
  - `arena-recorder/api/server.go`: Ensure `CanRecordScene` correctly assesses `publ` claims on the scene. (It currently checks `realm/s/<namespace>/<sceneId>/#` and `auth.CanRecordScene`).
  - `arena-web-core/scenes/scenes.js`: Before enabling the "Record" buttons (`recordUserSceneBtn` and `recordPublicSceneBtn`), parse the user's MQTT token or check permissions to verify they have publish rights. Wait, the token parsing can be done by parsing the JWT payload from `window.ARENAAUTH.mqtt_token`. If so, read `publ` topics and test against the scene. Otherwise, hide/disable the button.
  - `arena-web-core/src/systems/ui/icons.js` (if present): Conditionally render the record icon in the main scene UI based on `publ` permissions.

### 3. Replay Authorization Isolation
- **Action**: Enforce that only users with the `subl` right to the scene can view a recording of that scene.
- **Details**:
  - `arena-recorder/api/server.go`: Update `/recorder/list` and `/recorder/files/{filename}`.
    - `/recorder/list`: Get the user's `subl` claims from the JWT. Filter the list of recordings to only include those where the user's `subl` permissions match `realm/s/{namespace}/{sceneId}/#` (or they are admin).
    - `/recorder/files/`: Instead of `http.FileServer`, write a custom handler that checks the specific file's `namespace` and `sceneId` against the user's `subl` claims before serving the file.

### 4. Interactive Scene Recording Banner
- **Action**: When a scene is being recorded, notify all users in the scene with a persistent banner.
- **Details**:
  - `arena-recorder/mqtt/recorder.go`: When recording starts, publish a retained message to `realm/s/{namespace}/{sceneId}/!record` with `{"status": "recording"}`. When recording stops, publish `{"status": "stopped"}`.
  - `arena-web-core` (or a dedicated component/system): Subscribe to `realm/s/+/+/!record`. When a `{"status": "recording"}` message is received, emit an event or create an HTML banner indicating "Recording in Progress". This avoids a REST polling mechanism and relies on the pubsub efficiency.

### 5. Arena Account Profile Adjustments
- **Action**: Remove the general "record a scene" button from the user profile, but mark recorded scenes and list them under edit scene permissions.
- **Details**:
  - `arena-account/users/templates/users/user_profile.html`: Remove the `<button class="btn btn-danger btn-sm record-scene-btn">Record</button>`.
  - Instead, the backend (Django) can communicate with `arena-recorder` (or check the filesystem) to pass a `has_recordings` boolean for each scene to the template, placing an icon on the scene list.
  - Inside the "edit" scene view (`scene_permissions.html`?), list the specific recordings for that scene, providing links to the Replay UI (`/replay/?recording=...` or similar).

## Verification Plan
### Automated Verification
- No exhaustive test suites for `arena-recorder` exist yet. The Go compiler will verify syntax and typing.
- Run `docker-compose -f docker-compose.yaml build arena-recorder` to test builds.

### Manual Verification
- Deploy `arena-services-docker` locally using `docker-compose`.
- Open the ARENA account portal, verify the "Record" button is gone from the main profile page.
- Log in and verify if the scenes are listed properly.
- Open a scene in `arena-web-core` using a user with publish permissions. Attempt to record from the bottom UI menu or the `scenes` page.
- Verify the "Recording in Progress" banner appears for all clients connected to that scene.
- Load the `/replay` page and verify only authorized scenes appear in the dropdown.
- Directly query `/recorder/list` and `/recorder/files/...` using `curl` with a non-authorized JWT to ensure 403 Forbidden is returned.
