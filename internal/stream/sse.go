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
	"io"
	"strings"
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

	br := bufio.NewReader(r)
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

	for {
		line, err := br.ReadString('\n')
		// Process the line first (it may be a complete line without trailing
		// newline at EOF), then handle any error.
		if line != "" {
			// Strip CR and LF.
			line = strings.TrimRight(line, "\r\n")
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
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(value)
			default:
				// Comments (line starts with ':') and unknown fields are ignored.
			}
		}
		if err != nil {
			if err == io.EOF {
				// Flush any pending event at EOF (no trailing blank line).
				flush()
				return nil
			}
			return err
		}
	}
}
