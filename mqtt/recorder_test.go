package mqtt

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- deepMerge tests ---

func TestDeepMerge_BasicOverwrite(t *testing.T) {
	dst := map[string]interface{}{"a": "old", "b": 1.0}
	src := map[string]interface{}{"a": "new", "c": 2.0}
	result := deepMerge(dst, src)

	if result["a"] != "new" {
		t.Errorf("expected a=new, got %v", result["a"])
	}
	if result["b"] != 1.0 {
		t.Errorf("expected b=1, got %v", result["b"])
	}
	if result["c"] != 2.0 {
		t.Errorf("expected c=2, got %v", result["c"])
	}
}

func TestDeepMerge_NestedRecursion(t *testing.T) {
	dst := map[string]interface{}{
		"position": map[string]interface{}{"x": 1.0, "y": 2.0, "z": 3.0},
	}
	src := map[string]interface{}{
		"position": map[string]interface{}{"y": 99.0},
	}
	result := deepMerge(dst, src)

	pos, ok := result["position"].(map[string]interface{})
	if !ok {
		t.Fatal("position should be a map")
	}
	if pos["x"] != 1.0 {
		t.Errorf("expected x=1, got %v", pos["x"])
	}
	if pos["y"] != 99.0 {
		t.Errorf("expected y=99, got %v", pos["y"])
	}
	if pos["z"] != 3.0 {
		t.Errorf("expected z=3, got %v", pos["z"])
	}
}

func TestDeepMerge_NilDst(t *testing.T) {
	src := map[string]interface{}{"a": "hello"}
	result := deepMerge(nil, src)
	if result["a"] != "hello" {
		t.Errorf("expected a=hello, got %v", result["a"])
	}
}

func TestDeepMerge_ArrayOverwrite(t *testing.T) {
	dst := map[string]interface{}{
		"tags": []interface{}{"old1", "old2"},
	}
	src := map[string]interface{}{
		"tags": []interface{}{"new1"},
	}
	result := deepMerge(dst, src)
	tags, ok := result["tags"].([]interface{})
	if !ok {
		t.Fatal("tags should be a slice")
	}
	if len(tags) != 1 || tags[0] != "new1" {
		t.Errorf("expected tags=[new1], got %v", tags)
	}
}

func TestDeepMerge_TypeChangeOverwrite(t *testing.T) {
	// If dst has a string and src puts a map, src wins
	dst := map[string]interface{}{"data": "was_string"}
	src := map[string]interface{}{"data": map[string]interface{}{"nested": true}}
	result := deepMerge(dst, src)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data should now be a map")
	}
	if data["nested"] != true {
		t.Errorf("expected nested=true, got %v", data["nested"])
	}
}

// --- State tracking tests ---

// newTestSession creates a RecordingSession writing to a temp file, suitable for testing.
func newTestSession(t *testing.T) (*RecordingSession, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}
	return &RecordingSession{
		File:           file,
		Writer:         bufio.NewWriterSize(file, 64*1024),
		compactedState: make(map[string]map[string]interface{}),
		index:          make([]IndexEntry, 0),
	}, path
}

func marshalLine(t *testing.T, obj map[string]interface{}) string {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStateTracking_CreateAndUpdate(t *testing.T) {
	session, _ := newTestSession(t)
	defer session.File.Close()

	// Create an object
	create := map[string]interface{}{
		"object_id": "cube1",
		"action":    "create",
		"data":      map[string]interface{}{"position": map[string]interface{}{"x": 0.0, "y": 0.0, "z": 0.0}},
	}
	session.writeLine("2026-01-01T00:00:00Z", create, marshalLine(t, create))

	if _, exists := session.compactedState["cube1"]; !exists {
		t.Fatal("cube1 should exist in compactedState after create")
	}

	// Update only position.y
	update := map[string]interface{}{
		"object_id": "cube1",
		"action":    "update",
		"data":      map[string]interface{}{"position": map[string]interface{}{"y": 5.0}},
	}
	session.writeLine("2026-01-01T00:00:01Z", update, marshalLine(t, update))

	state := session.compactedState["cube1"]
	data, ok := state["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data should be a map")
	}
	pos, ok := data["position"].(map[string]interface{})
	if !ok {
		t.Fatal("position should be a map")
	}
	if pos["x"] != 0.0 {
		t.Errorf("x should still be 0, got %v", pos["x"])
	}
	if pos["y"] != 5.0 {
		t.Errorf("y should be 5, got %v", pos["y"])
	}
}

func TestStateTracking_Delete(t *testing.T) {
	session, _ := newTestSession(t)
	defer session.File.Close()

	create := map[string]interface{}{
		"object_id": "cube1",
		"action":    "create",
		"data":      map[string]interface{}{"color": "red"},
	}
	session.writeLine("2026-01-01T00:00:00Z", create, marshalLine(t, create))

	if _, exists := session.compactedState["cube1"]; !exists {
		t.Fatal("cube1 should exist")
	}

	del := map[string]interface{}{
		"object_id": "cube1",
		"action":    "delete",
	}
	session.writeLine("2026-01-01T00:00:01Z", del, marshalLine(t, del))

	if _, exists := session.compactedState["cube1"]; exists {
		t.Fatal("cube1 should NOT exist after delete")
	}
}

func TestStateTracking_NoObjectId(t *testing.T) {
	session, _ := newTestSession(t)
	defer session.File.Close()

	// System message without object_id — should not affect state
	msg := map[string]interface{}{
		"action": "update",
		"text":   "hello",
	}
	session.writeLine("2026-01-01T00:00:00Z", msg, marshalLine(t, msg))

	if len(session.compactedState) != 0 {
		t.Errorf("compactedState should be empty, has %d entries", len(session.compactedState))
	}
}

func TestStateTracking_NilPayload(t *testing.T) {
	session, _ := newTestSession(t)
	defer session.File.Close()

	// Fallback path: nil payload shouldn't panic
	session.writeLine("2026-01-01T00:00:00Z", nil, `{"raw":"stuff"}`)

	if len(session.compactedState) != 0 {
		t.Errorf("compactedState should be empty, has %d entries", len(session.compactedState))
	}
}

// --- Keyframe emission tests ---

func TestKeyframeEmission_WritesTrigger(t *testing.T) {
	session, path := newTestSession(t)

	// Create some objects
	for i := 0; i < 10; i++ {
		create := map[string]interface{}{
			"object_id": "obj" + string(rune('0'+i)),
			"action":    "create",
		}
		session.writeLine("2026-01-01T00:00:00Z", create, marshalLine(t, create))
	}

	// Write enough data to trigger a keyframe (>1MB)
	bigData := strings.Repeat("x", 256*1024) // 256KB per line
	for i := 0; i < 9; i++ {
		msg := map[string]interface{}{
			"object_id": "obj0",
			"action":    "update",
			"bigfield":  bigData,
		}
		session.writeLine("2026-01-01T00:00:01Z", msg, marshalLine(t, msg))
	}

	session.Writer.Flush()
	session.File.Close()

	if len(session.index) == 0 {
		t.Fatal("expected at least one keyframe to be emitted after >1MB")
	}

	// Verify the keyframe offset points to valid JSON
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	entry := session.index[0]
	buf := make([]byte, entry.Length)
	n, err := file.ReadAt(buf, entry.Offset)
	if err != nil {
		t.Fatalf("read at offset %d: %v", entry.Offset, err)
	}

	var kf map[string]interface{}
	if err := json.Unmarshal(buf[:n], &kf); err != nil {
		t.Fatalf("keyframe at offset %d is not valid JSON: %v", entry.Offset, err)
	}
	if kf["action"] != "keyframe" {
		t.Errorf("expected action=keyframe, got %v", kf["action"])
	}
	if kf["state"] == nil {
		t.Error("expected keyframe to contain state")
	}
}

func TestKeyframeEmission_IndexEntryHasLength(t *testing.T) {
	session, _ := newTestSession(t)
	defer session.File.Close()

	create := map[string]interface{}{
		"object_id": "obj0",
		"action":    "create",
	}
	session.writeLine("2026-01-01T00:00:00Z", create, marshalLine(t, create))

	// Force a keyframe
	session.writeMu.Lock()
	session.emitKeyframeLocked("2026-01-01T00:00:00Z")
	session.writeMu.Unlock()

	if len(session.index) != 1 {
		t.Fatalf("expected 1 index entry, got %d", len(session.index))
	}
	if session.index[0].Length <= 0 {
		t.Errorf("expected positive Length, got %d", session.index[0].Length)
	}
}

// --- Repair tests ---

func writeTestRecording(t *testing.T, dir string, lines []string) string {
	t.Helper()
	path := filepath.Join(dir, "test.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		file.WriteString(line)
		file.WriteString("\n")
	}
	file.Close()
	return path
}

func TestRepairIndex_AddsIndex(t *testing.T) {
	dir := t.TempDir()

	kf := `{"action":"keyframe","timestamp":"2026-01-01T00:00:00Z","state":{"obj1":{"object_id":"obj1"}}}`
	lines := []string{
		`{"object_id":"obj1","action":"create","timestamp":"2026-01-01T00:00:00Z"}`,
		kf,
		`{"object_id":"obj1","action":"update","timestamp":"2026-01-01T00:00:01Z","data":{"x":1}}`,
	}

	path := writeTestRecording(t, dir, lines)

	n, err := RepairIndex(path)
	if err != nil {
		t.Fatalf("RepairIndex failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 keyframe, got %d", n)
	}

	// Verify the file now has a keyframe_index as its last line
	has, err := HasKeyframeIndex(path)
	if err != nil {
		t.Fatalf("HasKeyframeIndex failed: %v", err)
	}
	if !has {
		t.Error("expected keyframe_index to be present after repair")
	}
}

func TestRepairIndex_Idempotent(t *testing.T) {
	dir := t.TempDir()

	kf := `{"action":"keyframe","timestamp":"2026-01-01T00:00:00Z","state":{}}`
	lines := []string{kf}

	path := writeTestRecording(t, dir, lines)

	// First repair
	n1, err := RepairIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 1 {
		t.Fatalf("expected 1 keyframe, got %d", n1)
	}

	info1, _ := os.Stat(path)
	size1 := info1.Size()

	// Second repair — should be a no-op
	n2, err := RepairIndex(path)
	if err != nil {
		t.Fatal(err)
	}

	info2, _ := os.Stat(path)
	size2 := info2.Size()

	if size2 != size1 {
		t.Errorf("repair was not idempotent: size changed from %d to %d", size1, size2)
	}
	_ = n2
}

func TestRepairIndex_NoKeyframes(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"object_id":"obj1","action":"create"}`,
		`{"object_id":"obj1","action":"update","data":{"x":1}}`,
	}
	path := writeTestRecording(t, dir, lines)

	n, err := RepairIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 keyframes for file with no keyframes, got %d", n)
	}

	has, _ := HasKeyframeIndex(path)
	if has {
		t.Error("should not add keyframe_index when there are no keyframes")
	}
}

func TestRepairIndex_CorrectOffsets(t *testing.T) {
	dir := t.TempDir()

	line1 := `{"object_id":"obj1","action":"create","timestamp":"2026-01-01T00:00:00Z"}`
	kfLine := `{"action":"keyframe","timestamp":"2026-01-01T00:00:00Z","state":{"obj1":{"object_id":"obj1"}}}`
	line3 := `{"object_id":"obj1","action":"update","timestamp":"2026-01-01T00:00:01Z"}`
	lines := []string{line1, kfLine, line3}

	path := writeTestRecording(t, dir, lines)

	RepairIndex(path)

	// Read the keyframe_index and verify offsets
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	var lastLine string
	for scanner.Scan() {
		lastLine = scanner.Text()
	}

	var idx struct {
		Action string       `json:"action"`
		Index  []IndexEntry `json:"index"`
	}
	if err := json.Unmarshal([]byte(lastLine), &idx); err != nil {
		t.Fatalf("could not parse index line: %v", err)
	}

	if len(idx.Index) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(idx.Index))
	}

	entry := idx.Index[0]
	expectedOffset := int64(len(line1) + 1) // line1 + \n
	if entry.Offset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, entry.Offset)
	}
	if entry.Length != int64(len(kfLine)) {
		t.Errorf("expected length %d, got %d", len(kfLine), entry.Length)
	}
}

// --- shallowDiff tests ---

func TestShallowDiff_NoChange(t *testing.T) {
	prev := map[string]interface{}{
		"arena-user": map[string]interface{}{"color": "#eca7ef", "displayName": "Ivan"},
		"position":   map[string]interface{}{"x": 1.0, "y": 1.6, "z": 5.0},
	}
	next := map[string]interface{}{
		"arena-user": map[string]interface{}{"color": "#eca7ef", "displayName": "Ivan"},
		"position":   map[string]interface{}{"x": 1.0, "y": 1.6, "z": 5.0},
	}
	delta := shallowDiff(prev, next)
	if len(delta) != 0 {
		t.Errorf("expected empty delta for identical data, got %v", delta)
	}
}

func TestShallowDiff_PartialPositionChange(t *testing.T) {
	prev := map[string]interface{}{
		"position": map[string]interface{}{"x": 1.0, "y": 1.6, "z": 5.0},
	}
	next := map[string]interface{}{
		"position": map[string]interface{}{"x": 2.0, "y": 1.6, "z": 5.2},
	}
	delta := shallowDiff(prev, next)
	pos, ok := delta["position"].(map[string]interface{})
	if !ok {
		t.Fatal("expected position in delta (value changed)")
	}
	// Shallow diff includes the entire position object atomically
	if pos["x"] != 2.0 {
		t.Errorf("expected x=2, got %v", pos["x"])
	}
	if pos["y"] != 1.6 {
		t.Errorf("expected y=1.6 (included even though unchanged), got %v", pos["y"])
	}
	if pos["z"] != 5.2 {
		t.Errorf("expected z=5.2, got %v", pos["z"])
	}
}

func TestShallowDiff_StaticSubObjectDropped(t *testing.T) {
	// arena-user block is identical — should be absent from delta
	arenaUser := map[string]interface{}{"color": "#eca7ef", "displayName": "Ivan", "presence": "Standard"}
	prev := map[string]interface{}{
		"arena-user": arenaUser,
		"position":   map[string]interface{}{"x": 1.0, "y": 1.6, "z": 5.0},
	}
	next := map[string]interface{}{
		"arena-user": arenaUser,
		"position":   map[string]interface{}{"x": 2.0, "y": 1.6, "z": 5.0},
	}
	delta := shallowDiff(prev, next)
	if _, hasArenaUser := delta["arena-user"]; hasArenaUser {
		t.Error("arena-user should NOT be in delta (unchanged)")
	}
	if _, hasPos := delta["position"]; !hasPos {
		t.Error("position should be in delta (x changed)")
	}
}

func TestShallowDiff_NewField(t *testing.T) {
	prev := map[string]interface{}{"position": map[string]interface{}{"x": 1.0}}
	next := map[string]interface{}{"position": map[string]interface{}{"x": 1.0}, "color": "red"}
	delta := shallowDiff(prev, next)
	if delta["color"] != "red" {
		t.Errorf("expected new field 'color' in delta, got %v", delta["color"])
	}
}

func TestShallowDiff_HeartbeatProducesEmptyDelta(t *testing.T) {
	// Full camera payload — nothing changed (pure TTL heartbeat scenario)
	data := map[string]interface{}{
		"arena-user": map[string]interface{}{
			"color": "#eca7ef", "displayName": "Ivanchrome2",
			"hasAudio": false, "hasVideo": false,
			"headModelPath": "/static/models/avatars/DamagedHelmet.glb",
			"jitsiId": "7210ee23", "presence": "Standard",
		},
		"object_type": "camera",
		"position":    map[string]interface{}{"x": 1.484, "y": 1.6, "z": 6.366},
		"rotation":    map[string]interface{}{"w": 0.893, "x": -0.06, "y": -0.446, "z": -0.03},
	}
	delta := shallowDiff(data, data)
	if len(delta) != 0 {
		t.Errorf("heartbeat should produce empty delta, got %v", delta)
	}
}

func TestShallowDiff_NullSemanticDelete(t *testing.T) {
	// Simulates data: {fooComponent: null} — a semantic delete of a component.
	// The null must flow through to the delta so the client removes the component.
	prev := map[string]interface{}{
		"fooComponent": map[string]interface{}{"bar": 1.0, "baz": "hello"},
		"position":     map[string]interface{}{"x": 1.0, "y": 2.0},
	}
	next := map[string]interface{}{
		"fooComponent": nil, // semantic delete
		"position":     map[string]interface{}{"x": 1.0, "y": 2.0},
	}
	delta := shallowDiff(prev, next)
	if _, hasFoo := delta["fooComponent"]; !hasFoo {
		t.Error("expected fooComponent in delta (set to null = semantic delete)")
	}
	if delta["fooComponent"] != nil {
		t.Errorf("expected fooComponent to be nil in delta, got %v", delta["fooComponent"])
	}
	if _, hasPos := delta["position"]; hasPos {
		t.Error("position should NOT be in delta (unchanged)")
	}
}

func TestShallowDiff_NullToNullUnchanged(t *testing.T) {
	// If a component was already null and arrives as null again, no change.
	prev := map[string]interface{}{"fooComponent": nil, "x": 1.0}
	next := map[string]interface{}{"fooComponent": nil, "x": 1.0}
	delta := shallowDiff(prev, next)
	if len(delta) != 0 {
		t.Errorf("expected empty delta (null→null is no change), got %v", delta)
	}
}

func TestShallowDiff_PrimitiveToNull(t *testing.T) {
	// A string/number field becoming null should appear in the delta.
	prev := map[string]interface{}{"label": "hello", "count": 5.0}
	next := map[string]interface{}{"label": nil, "count": 5.0}
	delta := shallowDiff(prev, next)
	if _, has := delta["label"]; !has {
		t.Error("expected label in delta (string→null)")
	}
	if delta["label"] != nil {
		t.Errorf("expected label to be nil, got %v", delta["label"])
	}
	if _, has := delta["count"]; has {
		t.Error("count should NOT be in delta (unchanged)")
	}
}

func TestWriteLine_NullComponentInDelta(t *testing.T) {
	session, path := newTestSession(t)

	// Create object with a component
	create := map[string]interface{}{
		"object_id": "cube1",
		"action":    "create",
		"type":      "object",
		"timestamp": "2026-01-01T00:00:00Z",
		"data": map[string]interface{}{
			"fooComponent": map[string]interface{}{"bar": 1.0},
			"position":     map[string]interface{}{"x": 0.0, "y": 0.0, "z": 0.0},
		},
	}
	session.writeLine("ts1", create, "")

	// Update: semantic delete of fooComponent via null, position unchanged
	update := map[string]interface{}{
		"object_id": "cube1",
		"action":    "update",
		"type":      "object",
		"timestamp": "2026-01-01T00:00:01Z",
		"data": map[string]interface{}{
			"fooComponent": nil,
			"position":     map[string]interface{}{"x": 0.0, "y": 0.0, "z": 0.0},
		},
	}
	session.writeLine("ts2", update, "")

	session.Writer.Flush()
	session.File.Close()

	// Read back the second line
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open temp file %q: %v", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	var written map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &written); err != nil {
		t.Fatalf("line 2 not valid JSON: %v", err)
	}

	data, ok := written["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data should be a map")
	}

	// fooComponent must be present and null (semantic delete must survive diffing)
	fooVal, hasFoo := data["fooComponent"]
	if !hasFoo {
		t.Error("fooComponent must be present in delta (semantic delete)")
	}
	if fooVal != nil {
		t.Errorf("fooComponent should be null in delta, got %v", fooVal)
	}

	// position must be absent (unchanged)
	if _, hasPos := data["position"]; hasPos {
		t.Error("position should be absent from delta (unchanged)")
	}
}

func TestWriteLine_DeltaCompression(t *testing.T) {
	session, path := newTestSession(t)

	// First message — create with full data
	full := func(x, z, rw float64) map[string]interface{} {
		return map[string]interface{}{
			"object_id": "Ivan_cam",
			"action":    "update",
			"type":      "object",
			"ttl":       30.0,
			"timestamp": "2026-04-16T20:37:22Z",
			"data": map[string]interface{}{
				"arena-user": map[string]interface{}{
					"color": "#eca7ef", "displayName": "Ivanchrome2",
					"hasAudio": false, "hasVideo": false,
					"headModelPath": "/static/models/avatars/DamagedHelmet.glb",
					"jitsiId": "7210ee23", "presence": "Standard",
				},
				"object_type": "camera",
				"position":    map[string]interface{}{"x": x, "y": 1.6, "z": z},
				"rotation":    map[string]interface{}{"w": rw, "x": -0.019, "y": 0.013, "z": 0.0},
			},
		}
	}

	// Seed state with a create (no diff on first occurrence)
	create := full(2.965, 8.877, 1.0)
	create["action"] = "create"
	session.writeLine("ts1", create, marshalLine(t, create))

	// Second message — position change only
	msg2 := full(2.852, 8.88, 1.0) // x and z change; rotation unchanged; arena-user unchanged
	session.writeLine("ts2", msg2, marshalLine(t, msg2))

	session.Writer.Flush()
	session.File.Close()

	// Read back the second line from the file
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	var written map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &written); err != nil {
		t.Fatalf("line 2 is not valid JSON: %v", err)
	}

	// Top-level fields must all be present
	for _, field := range []string{"object_id", "action", "type", "ttl", "timestamp"} {
		if _, ok := written[field]; !ok {
			t.Errorf("expected top-level field %q to be present", field)
		}
	}

	data, ok := written["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data should be a map")
	}

	// arena-user must be absent (unchanged)
	if _, hasArenaUser := data["arena-user"]; hasArenaUser {
		t.Error("arena-user should be absent from delta (unchanged)")
	}
	// position must be present (changed)
	if _, hasPos := data["position"]; !hasPos {
		t.Error("position should be in delta (x+z changed)")
	}
	// rotation must be absent (unchanged)
	if _, hasRot := data["rotation"]; hasRot {
		t.Error("rotation should be absent from delta (unchanged)")
	}
}

// --- Benchmarks ---

// cameraData returns a realistic camera update data payload.
func cameraData(x, z, rw, rx, ry float64) map[string]interface{} {
	return map[string]interface{}{
		"arena-user": map[string]interface{}{
			"color": "#eca7ef", "displayName": "Ivanchrome2",
			"hasAudio": false, "hasVideo": false,
			"headModelPath": "/static/models/avatars/DamagedHelmet.glb",
			"jitsiId": "7210ee23", "presence": "Standard",
		},
		"object_type": "camera",
		"position":    map[string]interface{}{"x": x, "y": 1.6, "z": z},
		"rotation":    map[string]interface{}{"w": rw, "x": rx, "y": ry, "z": 0.0},
	}
}

// BenchmarkShallowDiff measures pure delta computation with a realistic camera payload.
// Most of `data` is static (arena-user, object_type); only position/rotation change.
func BenchmarkShallowDiff(b *testing.B) {
	prev := cameraData(2.965, 8.877, 1.0, -0.019, 0.013)
	next := cameraData(2.852, 8.88, 0.959, -0.065, -0.276)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shallowDiff(prev, next)
	}
}

// BenchmarkShallowDiff_NoDiff measures the cost when nothing has changed (heartbeat path).
func BenchmarkShallowDiff_NoDiff(b *testing.B) {
	data := cameraData(2.965, 8.877, 1.0, -0.019, 0.013)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shallowDiff(data, data)
	}
}

// BenchmarkJSONValEqual measures primitive comparison cost.
func BenchmarkJSONValEqual(b *testing.B) {
	b.Run("equal_float", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			jsonValEqual(float64(1.484), float64(1.484))
		}
	})
	b.Run("unequal_float", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			jsonValEqual(float64(1.484), float64(2.852))
		}
	})
	b.Run("equal_bool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			jsonValEqual(false, false)
		}
	})
}

// BenchmarkWriteLine_WithDiff measures the full writeLine path with delta compression.
// It writes to a buffered temporary file, so results reflect the current file-backed path.
func BenchmarkWriteLine_WithDiff(b *testing.B) {
	// Use a temp file; OS buffering and the write cache keep most writes in-memory.
	dir := b.TempDir()
	file, err := os.OpenFile(dir+"/bench.jsonl", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	session := &RecordingSession{
		File:           file,
		Writer:         bufio.NewWriterSize(file, 64*1024),
		compactedState: make(map[string]map[string]interface{}),
		index:          make([]IndexEntry, 0),
	}

	// Seed with initial state
	seed := map[string]interface{}{
		"object_id": "Ivan_cam", "action": "create", "type": "object", "ttl": 30.0,
		"timestamp": "2026-04-16T20:37:22Z",
		"data":      cameraData(2.965, 8.877, 1.0, -0.019, 0.013),
	}
	session.writeLine("ts0", seed, "")

	// Camera positions from the sample — cycle through them
	frames := [][5]float64{
		{2.852, 8.880, 1.000, -0.019, 0.013},
		{2.539, 8.886, 1.000, -0.026, -0.008},
		{2.137, 8.773, 0.976, -0.055, -0.208},
		{1.814, 8.368, 0.959, -0.065, -0.276},
		{1.581, 7.786, 0.951, -0.065, -0.301},
		{1.455, 7.103, 0.923, -0.059, -0.380},
		{1.484, 6.366, 0.893, -0.060, -0.446},
		{1.577, 5.632, 0.876, -0.064, -0.476},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset byte counters to prevent keyframe emission from skewing results.
		session.bytesWritten = 0
		session.lastKeyframeBytes = 0
		f := frames[i%len(frames)]
		payload := map[string]interface{}{
			"object_id": "Ivan_cam", "action": "update", "type": "object", "ttl": 30.0,
			"timestamp": "2026-04-16T20:37:23Z",
			"data":      cameraData(f[0], f[1], f[2], f[3], f[4]),
		}
		session.writeLine("ts", payload, "")
	}
}

// BenchmarkWriteLine_NoDiff is the baseline: same path but no prior state,
// so delta compression is bypassed (first-message / create path).
func BenchmarkWriteLine_NoDiff(b *testing.B) {
	dir := b.TempDir()
	file, err := os.OpenFile(dir+"/bench_nodiff.jsonl", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	session := &RecordingSession{
		File:           file,
		Writer:         bufio.NewWriterSize(file, 64*1024),
		compactedState: make(map[string]map[string]interface{}),
		index:          make([]IndexEntry, 0),
	}

	frames := [][5]float64{
		{2.852, 8.880, 1.000, -0.019, 0.013},
		{2.539, 8.886, 1.000, -0.026, -0.008},
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Each iteration resets compactedState so there's never prior state for delta diffing.
	for i := 0; i < b.N; i++ {
		f := frames[i%len(frames)]
		payload := map[string]interface{}{
			"object_id": "fresh_obj", "action": "create", "type": "object", "ttl": 30.0,
			"timestamp": "2026-04-16T20:37:23Z",
			"data":      cameraData(f[0], f[1], f[2], f[3], f[4]),
		}
		session.compactedState = make(map[string]map[string]interface{}) // reset state each iter
		session.writeLine("ts", payload, "")
	}
}

// TestWriteLine_SizeReduction measures actual bytes written to disk for the
// exact 9-message camera sequence from the sample, comparing compressed vs raw.
func TestWriteLine_SizeReduction(t *testing.T) {
	// The 9 sample messages verbatim (as raw strings so we get exact byte counts)
	rawMessages := []string{
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":2.965,"y":1.6,"z":8.877},"rotation":{"w":1,"x":-0.019,"y":0.013,"z":0}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:22.794356099Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":2.852,"y":1.6,"z":8.88},"rotation":{"w":1,"x":-0.019,"y":0.013,"z":0}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:23.733577767Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":2.539,"y":1.6,"z":8.886},"rotation":{"w":1,"x":-0.026,"y":-0.008,"z":0}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:23.836278491Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":2.137,"y":1.6,"z":8.773},"rotation":{"w":0.976,"x":-0.055,"y":-0.208,"z":-0.012}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:23.940357863Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":1.814,"y":1.6,"z":8.368},"rotation":{"w":0.959,"x":-0.065,"y":-0.276,"z":-0.019}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:24.044548608Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":1.581,"y":1.6,"z":7.786},"rotation":{"w":0.951,"x":-0.065,"y":-0.301,"z":-0.021}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:24.148821046Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":1.455,"y":1.6,"z":7.103},"rotation":{"w":0.923,"x":-0.059,"y":-0.38,"z":-0.024}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:24.252849007Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":1.484,"y":1.6,"z":6.366},"rotation":{"w":0.893,"x":-0.06,"y":-0.446,"z":-0.03}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:24.357174715Z","ttl":30,"type":"object"}`,
		`{"action":"update","data":{"arena-user":{"color":"#eca7ef","displayName":"Ivanchrome2","hasAudio":false,"hasVideo":false,"headModelPath":"/static/models/avatars/DamagedHelmet.glb","jitsiId":"7210ee23","presence":"Standard"},"object_type":"camera","position":{"x":1.577,"y":1.6,"z":5.632},"rotation":{"w":0.876,"x":-0.064,"y":-0.476,"z":-0.035}},"object_id":"Ivan_2268485826","timestamp":"2026-04-16T20:37:24.461516815Z","ttl":30,"type":"object"}`,
	}

	// Uncompressed baseline: sum of raw message lengths + newlines
	uncompressedBytes := 0
	for _, m := range rawMessages {
		uncompressedBytes += len(m) + 1 // +1 for \n
	}

	// Compressed: run through writeLine with delta enabled
	session, path := newTestSession(t)
	for _, raw := range rawMessages {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("bad fixture JSON: %v", err)
		}
		session.writeLine("ts", payload, raw)
	}
	session.Writer.Flush()
	session.File.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	compressedBytes := int(info.Size())

	saving := 100.0 * (1.0 - float64(compressedBytes)/float64(uncompressedBytes))
	t.Logf("Uncompressed : %d bytes", uncompressedBytes)
	t.Logf("Compressed   : %d bytes", compressedBytes)
	t.Logf("Saving       : %.1f%%", saving)

	// Read back compressed lines so we can log them
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open compressed file: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	i := 0
	for scanner.Scan() {
		t.Logf("  [%d] %d bytes: %s", i+1, len(scanner.Text()), scanner.Text())
		i++
	}
	file.Close()

	if saving < 30 {
		t.Errorf("expected at least 30%% size reduction, got %.1f%%", saving)
	}
}

func TestHasKeyframeIndex_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte{}, 0644)

	has, err := HasKeyframeIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("empty file should not have keyframe_index")
	}
}
