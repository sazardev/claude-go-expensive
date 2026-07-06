package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// maxLineSize bounds a single transcript line. Tool output embedded in a
// message can be large, so this is much bigger than bufio.Scanner's default.
const maxLineSize = 16 * 1024 * 1024

// ReadFile parses a Claude Code session JSONL transcript.
func ReadFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()
	return Read(f)
}

// Read parses a Claude Code session JSONL transcript from r. Lines that fail
// to parse as JSON are skipped — Claude Code appends to the active session
// file live, so the last line can be a partial write.
func Read(r io.Reader) ([]Entry, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var entries []Entry
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scan transcript: %w", err)
	}
	return entries, nil
}
