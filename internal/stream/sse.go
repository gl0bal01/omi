// Package stream parses Server-Sent Events (SSE) for the chat endpoint.
//
// We hand-roll the parser to avoid an external dependency. Wire format:
//
//	event: <name>\n
//	data: <payload>\n
//	\n          # blank line terminates an event
//
// Multiple `data:` lines within one event are joined with `\n`.
// A leading single space after the colon is stripped per the SSE spec.
// Lines without a colon are skipped silently.
//
// Recognized event names map to Event.Type:
//   - "content"         (default for unnamed/"message" events too)
//   - "result"          (caller usually ignores)
//   - "done"            (terminates the stream)
//   - "error"           (terminates the stream)
package stream

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	// maxStreamBytes caps the total SSE response read so a hostile or
	// compromised upstream cannot stream unbounded bytes into memory.
	maxStreamBytes = 64 << 20 // 64 MiB
	// maxLineBytes caps a single SSE line so a server that omits newlines
	// cannot grow the scanner buffer without limit.
	maxLineBytes = 1 << 20 // 1 MiB
	// maxEventBytes caps one event's accumulated data: payload.
	maxEventBytes = 16 << 20 // 16 MiB
)

// Event is one decoded SSE event.
type Event struct {
	Type string
	Data string
}

// Event type constants returned in Event.Type.
const (
	EventContent = "content"
	EventDone    = "done"
	EventError   = "error"
	EventResult  = "result"
)

// Parse reads SSE bytes from r and emits events on out. It closes out before
// returning. It returns nil on EOF, on `event: done`, or on `event: error`.
// Read errors (other than io.EOF) are returned to the caller.
func Parse(r io.Reader, out chan<- Event) error {
	defer close(out)

	// Bound the total bytes read and the size of any single line so a hostile
	// upstream cannot exhaust memory. ScanLines strips the trailing newline
	// (and a trailing CR) for us.
	sc := bufio.NewScanner(io.LimitReader(r, maxStreamBytes))
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	var (
		evType  string
		dataBuf strings.Builder
	)

	flush := func() (stop bool) {
		if evType == "" && dataBuf.Len() == 0 {
			return false
		}
		t := evType
		if t == "" || t == "message" {
			t = EventContent
		}
		ev := Event{Type: t, Data: dataBuf.String()}
		out <- ev
		evType = ""
		dataBuf.Reset()
		return t == EventDone || t == EventError
	}

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			// Blank line: dispatch event.
			if flush() {
				return nil
			}
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			// Malformed line — skip silently.
			continue
		}
		field := line[:colon]
		value := line[colon+1:]
		// Per SSE spec: a single optional leading space after the colon
		// is stripped.
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch field {
		case "event":
			evType = value
		case "data":
			add := len(value)
			if dataBuf.Len() > 0 {
				add++ // joining newline
			}
			if dataBuf.Len()+add > maxEventBytes {
				return fmt.Errorf("sse: event exceeded %d bytes", maxEventBytes)
			}
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(value)
		default:
			// Comments (line starts with ':') and unknown fields are ignored.
		}
	}
	if err := sc.Err(); err != nil {
		// bufio.ErrTooLong (line exceeded maxLineBytes) surfaces here too.
		return err
	}
	// Flush any pending event at EOF (no trailing blank line).
	flush()
	return nil
}
