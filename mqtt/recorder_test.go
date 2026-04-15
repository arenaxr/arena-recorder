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
