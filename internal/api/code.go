// CODE_GENERATOR remains on /api/features per official 1min.ai docs (not in deprecation list).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gl0bal01/omi/internal/closeutil"
)

// CodeRequest is the body for POST /api/features with CODE_GENERATOR.
type CodeRequest struct {
	Type         string           `json:"type"`
	Model        string           `json:"model"`
	PromptObject codePromptObject `json:"promptObject"`
}

type codePromptObject struct {
	Prompt string `json:"prompt"`
}

// Code submits a code-generation request and returns the generated text.
// Non-streaming per the routing table.
func (c *Client) Code(ctx context.Context, model, prompt string) (string, error) {
	body := CodeRequest{
		Type:  "CODE_GENERATOR",
		Model: model,
		PromptObject: codePromptObject{
			Prompt: prompt,
		},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/features", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(buf)), nil }

	resp, err := c.do(req, false)
	if err != nil {
		return "", err
	}
	defer closeutil.Quiet(resp.Body)
	if resp.StatusCode >= 400 {
		msg := readErrorBody(resp.Body)
		return "", &Error{Status: resp.StatusCode, Message: string(msg)}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return "", err
	}
	text := extractFeatureResult(raw)
	if text == "" {
		return "", fmt.Errorf("omi: code response missing result text")
	}
	return text, nil
}
