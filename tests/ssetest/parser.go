// Package ssetest provides a minimal SSE frame parser shared by unit
// and integration tests. It is intentionally side-effect free: errors are
// returned, never logged, so each caller can map them to t.Fatalf with its
// own context.
package ssetest

import (
	"bufio"
	"io"
	"strings"
)

// Frame is one parsed SSE frame. Event is the value after `event: `; Data is
// the concatenated value of one or more `data: ` lines, joined by '\n' to
// match the SSE wire shape.
type Frame struct {
	Event string
	Data  string
}

// ParseAll reads r to EOF and returns every frame in order. A frame is
// closed by an empty line or end-of-stream. Lines that do not start with
// `event:` or `data:` are ignored.
func ParseAll(r io.Reader) ([]Frame, error) {
	frames, _, err := parse(r, "")
	return frames, err
}

// ReadUntilEvent reads r until a frame with Event == stopAt is appended or
// the stream ends. The stopping frame is included in the returned slice.
// Useful when the test wants to halt on a terminal event (e.g. summary)
// without draining further bytes.
func ReadUntilEvent(r io.Reader, stopAt string) ([]Frame, error) {
	frames, _, err := parse(r, stopAt)
	return frames, err
}

func parse(r io.Reader, stopAt string) ([]Frame, bool, error) {
	scanner := bufio.NewScanner(r)
	// Default 64K buffer is enough today, but JSON payloads can grow; cap
	// at 1MiB so a single oversized frame fails fast instead of silently
	// truncating.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var (
		frames   []Frame
		curEvent string
		curData  strings.Builder
		stopped  bool
	)
	flush := func() {
		if curEvent == "" && curData.Len() == 0 {
			return
		}
		frames = append(frames, Frame{Event: curEvent, Data: curData.String()})
		if stopAt != "" && curEvent == stopAt {
			stopped = true
		}
		curEvent = ""
		curData.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			if stopped {
				return frames, true, nil
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if curData.Len() > 0 {
				curData.WriteByte('\n')
			}
			curData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	return frames, stopped, scanner.Err()
}
