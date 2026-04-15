package mqtt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// RepairIndex scans a .jsonl recording file for inline keyframe lines and
// appends a keyframe_index line at EOF if one is missing. This recovers
// recordings from unclean shutdowns where StopRecording never ran.
// If the last line is not valid JSON (partial write from crash), it is
// truncated before the index is appended.
//
// Returns the number of keyframes found, or an error.
func RepairIndex(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	// Scan the entire file line-by-line, tracking byte offsets
	var index []IndexEntry
	var offset int64
	hasExistingIndex := false
	// Track the last line for partial-write detection
	var lastLineOffset int64
	var lastLineValid bool
	var lastLineLen int64

	scanner := bufio.NewScanner(file)
	// Allow up to 64MB per line (keyframes with large state can be big)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		lineLen := int64(len(line))
		lastLineOffset = offset
		lastLineLen = lineLen

		// Check if this line is valid JSON
		lastLineValid = json.Valid([]byte(line))

		// Quick prefix check to avoid unmarshalling every line
		if lastLineValid && len(line) > 20 {
			var probe struct {
				Action string `json:"action"`
			}
			if json.Unmarshal([]byte(line), &probe) == nil {
				if probe.Action == "keyframe" {
					index = append(index, IndexEntry{
						Offset: offset,
						Length: lineLen,
					})
					// Extract the timestamp for the index entry
					var kf struct {
						Timestamp string `json:"timestamp"`
					}
					if json.Unmarshal([]byte(line), &kf) == nil {
						index[len(index)-1].Timestamp = kf.Timestamp
					}
				} else if probe.Action == "keyframe_index" {
					hasExistingIndex = true
				}
			}
		}

		// +1 for the \n that scanner stripped
		offset += lineLen + 1
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan error: %w", err)
	}

	if hasExistingIndex {
		log.Printf("File %s already has a keyframe_index, no repair needed (%d keyframes found)", path, len(index))
		return len(index), nil
	}

	// Truncate partial last line if it's not valid JSON
	if lastLineLen > 0 && !lastLineValid {
		log.Printf("Truncating partial last line at offset %d (%d bytes) in %s", lastLineOffset, lastLineLen, path)
		if err := os.Truncate(path, lastLineOffset); err != nil {
			return 0, fmt.Errorf("could not truncate partial line: %w", err)
		}
	}

	if len(index) == 0 {
		log.Printf("File %s has no keyframes — nothing to index", path)
		return 0, nil
	}

	// Append the index line
	indexLine := map[string]interface{}{
		"action": "keyframe_index",
		"index":  index,
	}
	b, err := json.Marshal(indexLine)
	if err != nil {
		return 0, fmt.Errorf("could not marshal index: %w", err)
	}

	appendFile, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("could not open file for append: %w", err)
	}
	defer appendFile.Close()

	if _, err := appendFile.Write(b); err != nil {
		return 0, fmt.Errorf("could not write index: %w", err)
	}
	if _, err := appendFile.WriteString("\n"); err != nil {
		return 0, fmt.Errorf("could not write newline: %w", err)
	}
	if err := appendFile.Sync(); err != nil {
		return 0, fmt.Errorf("could not sync: %w", err)
	}

	log.Printf("Repaired %s: wrote keyframe_index with %d entries", path, len(index))
	return len(index), nil
}

// RepairAllRecordings scans the recording store directory and repairs any
// .jsonl files missing a keyframe_index.
func RepairAllRecordings(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", dir, err)
	}

	repaired := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 6 || name[len(name)-6:] != ".jsonl" {
			continue
		}

		path := dir + "/" + name
		n, err := RepairIndex(path)
		if err != nil {
			log.Printf("Error repairing %s: %v", name, err)
			continue
		}
		if n > 0 {
			repaired++
		}
	}

	log.Printf("Repair complete: processed %d files in %s", repaired, dir)
	return nil
}

// HasKeyframeIndex checks whether a .jsonl file has a keyframe_index as its
// last line. Useful for quickly checking file health without full repair.
func HasKeyframeIndex(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Read last 4KB — the keyframe_index line should be well within this
	info, err := file.Stat()
	if err != nil {
		return false, err
	}

	readSize := int64(4096)
	if info.Size() < readSize {
		readSize = info.Size()
	}

	buf := make([]byte, readSize)
	_, err = file.ReadAt(buf, info.Size()-readSize)
	if err != nil && err != io.EOF {
		return false, err
	}

	// Find the last complete line
	tail := string(buf)
	// Trim trailing newline(s)
	for len(tail) > 0 && tail[len(tail)-1] == '\n' {
		tail = tail[:len(tail)-1]
	}
	lastNewline := -1
	for i := len(tail) - 1; i >= 0; i-- {
		if tail[i] == '\n' {
			lastNewline = i
			break
		}
	}
	var lastLine string
	if lastNewline >= 0 {
		lastLine = tail[lastNewline+1:]
	} else {
		lastLine = tail
	}

	var probe struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(lastLine), &probe); err != nil {
		return false, nil
	}
	return probe.Action == "keyframe_index", nil
}
