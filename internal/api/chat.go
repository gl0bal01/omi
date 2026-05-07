package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// ChatRequest is the body for POST /api/chat-with-ai. Type is always
// "UNIFY_CHAT_WITH_AI"; conversationId is camelCase; promptObject supports
// both legacy flat keys and nested settings for backend compatibility.
type ChatRequest struct {
	Type           string       `json:"type"`
	Model          string       `json:"model"`
	ConversationID string       `json:"conversationId,omitempty"` // legacy
	PromptObject   PromptObject `json:"promptObject"`
	ImageList      []string     `json:"imageList,omitempty"` // legacy
	Files          []string     `json:"files,omitempty"`     // legacy
}

type PromptObject struct {
	Prompt         string             `json:"prompt"`
	ConversationID string             `json:"conversationId,omitempty"`
	WebSearch      bool               `json:"webSearch,omitempty"` // legacy
	NumOfSite      int                `json:"numOfSite,omitempty"` // legacy
	MaxWord        int                `json:"maxWord,omitempty"`   // legacy
	IsMixed        bool               `json:"isMixed,omitempty"`   // legacy
	Settings       *PromptSettings    `json:"settings,omitempty"`
	Attachments    *PromptAttachments `json:"attachments,omitempty"`
}

type PromptSettings struct {
	WebSearchSettings *PromptWebSearchSettings `json:"webSearchSettings,omitempty"`
	HistorySettings   *PromptHistorySettings   `json:"historySettings,omitempty"`
}

type PromptWebSearchSettings struct {
	WebSearch bool `json:"webSearch,omitempty"`
	NumOfSite int  `json:"numOfSite,omitempty"`
	MaxWord   int  `json:"maxWord,omitempty"`
}

type PromptHistorySettings struct {
	IsMixed      bool `json:"isMixed,omitempty"`      // legacy
	HistoryMixed bool `json:"historyMixed,omitempty"` // forward-compat
	// Some upstream clients/docs used snake_case variants during rollout.
	HistoryMixedSnake bool `json:"history_mixed,omitempty"`
}

type PromptAttachments struct {
	Images []string `json:"images,omitempty"`
	Files  []string `json:"files,omitempty"`
}

// Chat opens a streaming SSE response from POST /api/chat-with-ai?isStreaming=true.
// The caller owns and must Close the returned body. Streaming requests are
// not retried (see Client.do).
func (c *Client) Chat(ctx context.Context, req *ChatRequest) (io.ReadCloser, error) {
	buf, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat-with-ai?isStreaming=true", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.do(httpReq, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		msg := readErrorBody(resp.Body)
		_ = resp.Body.Close()
		return nil, &Error{Status: resp.StatusCode, Message: string(msg)}
	}
	return resp.Body, nil
}
