package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestConsumeStream_JSONContentChunks_StreamMode(t *testing.T) {
	in := strings.Join([]string{
		"event: content",
		"data: {\"content\":\"\"}",
		"",
		"event: content",
		"data: {\"content\":\"Hello\"}",
		"",
		"event: content",
		"data: {\"content\":\"!\"}",
		"",
		"event: done",
		"",
		"",
	}, "\n")
	var out bytes.Buffer
	if err := consumeStream(strings.NewReader(in), false, &out, &out); err != nil {
		t.Fatalf("consumeStream: %v", err)
	}
	if got, want := out.String(), "Hello!\n"; got != want {
		t.Fatalf("out=%q want %q", got, want)
	}
}

func TestConsumeStream_JSONContentChunks_NoStreamMode(t *testing.T) {
	in := strings.Join([]string{
		"event: content",
		"data: {\"content\":\"Hello\"}",
		"",
		"event: content",
		"data: {\"content\":\" world\"}",
		"",
		"event: done",
		"",
		"",
	}, "\n")
	var out bytes.Buffer
	if err := consumeStream(strings.NewReader(in), true, &out, &out); err != nil {
		t.Fatalf("consumeStream: %v", err)
	}
	if got, want := out.String(), "Hello world\n"; got != want {
		t.Fatalf("out=%q want %q", got, want)
	}
}

func TestConsumeStream_PlainTextContentFallback(t *testing.T) {
	in := "event: content\ndata: hello\n\nevent: done\n\n"
	var out bytes.Buffer
	if err := consumeStream(strings.NewReader(in), false, &out, &out); err != nil {
		t.Fatalf("consumeStream: %v", err)
	}
	if got, want := out.String(), "hello\n"; got != want {
		t.Fatalf("out=%q want %q", got, want)
	}
}

func TestConsumeStream_ErrorJSONMessage(t *testing.T) {
	in := "event: error\ndata: {\"message\":\"bad request\"}\n\n"
	var out bytes.Buffer
	err := consumeStream(strings.NewReader(in), false, &out, &out)
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("err=%v", err)
	}
}

func TestConsumeStream_ErrorMessageSanitized(t *testing.T) {
	in := "event: error\ndata: {\"message\":\"bad\\u001b[31mred\"}\n\n"
	var out bytes.Buffer
	err := consumeStream(strings.NewReader(in), false, &out, &out)
	if err == nil {
		t.Fatalf("want error")
	}
	if strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("err leaked escape byte: %q", err.Error())
	}
	if !strings.Contains(err.Error(), `bad\x1B[31mred`) {
		t.Fatalf("err=%q", err.Error())
	}
}

func TestConsumeStream_ContentChunksSanitized(t *testing.T) {
	in := "event: content\ndata: {\"content\":\"hello\\u001b[31mred\\nnext\"}\n\nevent: done\n\n"
	var out bytes.Buffer
	if err := consumeStream(strings.NewReader(in), false, &out, &out); err != nil {
		t.Fatalf("consumeStream: %v", err)
	}
	if got, want := out.String(), "hello\\x1B[31mred\nnext\n"; got != want {
		t.Fatalf("out=%q want %q", got, want)
	}
}
