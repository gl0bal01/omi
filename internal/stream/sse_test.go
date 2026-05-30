package stream

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func collect(t *testing.T, r io.Reader) []Event {
	t.Helper()
	ch := make(chan Event, 16)
	errCh := make(chan error, 1)
	go func() { errCh <- Parse(r, ch) }()
	var got []Event
	for ev := range ch {
		got = append(got, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return got
}

func TestParse_SingleEvent(t *testing.T) {
	in := "event: content\ndata: hello\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	if got[0].Type != "content" || got[0].Data != "hello" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestParse_MultiEvent(t *testing.T) {
	in := "event: content\ndata: a\n\nevent: content\ndata: b\n\nevent: done\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d: %+v", len(got), got)
	}
	if got[0].Data != "a" || got[1].Data != "b" {
		t.Fatalf("data: %q %q", got[0].Data, got[1].Data)
	}
	if got[2].Type != "done" {
		t.Fatalf("want done, got %q", got[2].Type)
	}
}

func TestParse_SplitAcrossChunks(t *testing.T) {
	in := "event: content\ndata: hello world\n\nevent: done\n\n"
	// iotest.OneByteReader forces byte-by-byte reads.
	got := collect(t, iotest.OneByteReader(strings.NewReader(in)))
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	if got[0].Data != "hello world" {
		t.Fatalf("data=%q", got[0].Data)
	}
	if got[1].Type != "done" {
		t.Fatalf("want done")
	}
}

func TestParse_LeadingSpaceStripped(t *testing.T) {
	// One leading space stripped; second space preserved.
	in := "data:  hello\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event")
	}
	if got[0].Data != " hello" {
		t.Fatalf("data=%q (want %q)", got[0].Data, " hello")
	}
}

func TestParse_DefaultTypeIsContent(t *testing.T) {
	in := "data: hi\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event")
	}
	if got[0].Type != "content" {
		t.Fatalf("type=%q", got[0].Type)
	}
}

func TestParse_MultiDataConcat(t *testing.T) {
	in := "event: content\ndata: line1\ndata: line2\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event")
	}
	if got[0].Data != "line1\nline2" {
		t.Fatalf("data=%q", got[0].Data)
	}
}

func TestParse_MalformedLineSkipped(t *testing.T) {
	in := "this-is-not-a-field\nevent: content\ndata: ok\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	if got[0].Data != "ok" {
		t.Fatalf("data=%q", got[0].Data)
	}
}

func TestParse_DoneTerminates(t *testing.T) {
	// Bytes after `done` must not be emitted.
	in := "event: done\n\nevent: content\ndata: ignored\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	if got[0].Type != "done" {
		t.Fatalf("type=%q", got[0].Type)
	}
}

func TestParse_ErrorEvent(t *testing.T) {
	in := "event: error\ndata: boom\n\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event")
	}
	if got[0].Type != "error" || got[0].Data != "boom" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestParse_CRLF(t *testing.T) {
	in := "event: content\r\ndata: hello\r\n\r\n"
	got := collect(t, strings.NewReader(in))
	if len(got) != 1 {
		t.Fatalf("want 1 event")
	}
	if got[0].Data != "hello" {
		t.Fatalf("data=%q", got[0].Data)
	}
}

func TestParse_ChunkedBuffer(t *testing.T) {
	// Build a multi-event payload then deliver in 3-byte chunks.
	full := "event: content\ndata: aaa\n\nevent: content\ndata: bbb\n\nevent: done\n\n"
	got := collect(t, &chunkReader{src: []byte(full), n: 3})
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d", len(got))
	}
	if got[0].Data != "aaa" || got[1].Data != "bbb" {
		t.Fatalf("got %q %q", got[0].Data, got[1].Data)
	}
}

type chunkReader struct {
	src []byte
	off int
	n   int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.off >= len(c.src) {
		return 0, io.EOF
	}
	end := c.off + c.n
	if end > len(c.src) {
		end = len(c.src)
	}
	n := copy(p, c.src[c.off:end])
	c.off += n
	return n, nil
}

func TestParse_RejectsOversizedLine(t *testing.T) {
	// A single line far larger than maxLineBytes with no newline must error
	// (bufio.ErrTooLong) instead of buffering unbounded into memory.
	huge := "data: " + strings.Repeat("A", maxLineBytes+1)
	ch := make(chan Event, 4)
	if err := Parse(strings.NewReader(huge), ch); err == nil {
		t.Fatalf("expected error for oversized line, got nil")
	}
}

func TestParse_RejectsOversizedEvent(t *testing.T) {
	// Many bounded data: lines whose joined size exceeds maxEventBytes must
	// error rather than grow the event buffer without limit.
	var b strings.Builder
	line := "data: " + strings.Repeat("B", 64<<10) + "\n"
	for b.Len() < maxEventBytes+(64<<10) {
		b.WriteString(line)
	}
	ch := make(chan Event, 4)
	if err := Parse(strings.NewReader(b.String()), ch); err == nil {
		t.Fatalf("expected error for oversized event, got nil")
	}
}
