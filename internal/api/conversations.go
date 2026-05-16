package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gl0bal01/omi/internal/closeutil"
)

const (
	ConversationTypeChat  = "CHAT_WITH_AI"
	ConversationTypeUnify = "UNIFY_CHAT_WITH_AI"
)

// createConversationRequest mirrors /api/conversations payload.
type createConversationRequest struct {
	Title string `json:"title"`
	Type  string `json:"type"`
	Model string `json:"model"`
}

type createConversationResponse struct {
	Conversation struct {
		UUID string `json:"uuid"`
	} `json:"conversation"`
}

// CreateConversation POSTs /api/conversations and returns the UUID.
//
// On 400/422 validation failures, this method retries once with the alternate
// conversation type marker (UNIFY_CHAT_WITH_AI <-> CHAT_WITH_AI) to preserve
// compatibility across backend revisions.
func (c *Client) CreateConversation(ctx context.Context, title, convType, model string) (string, error) {
	uuid, err := c.createConversationOnce(ctx, title, convType, model)
	if err == nil {
		return uuid, nil
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || (apiErr.Status != http.StatusBadRequest && apiErr.Status != http.StatusUnprocessableEntity) {
		return "", err
	}

	alt := alternateConversationType(convType)
	if alt == "" || alt == convType {
		return "", err
	}
	return c.createConversationOnce(ctx, title, alt, model)
}

func (c *Client) createConversationOnce(ctx context.Context, title, convType, model string) (string, error) {
	body := createConversationRequest{Title: title, Type: convType, Model: model}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/conversations", bytes.NewReader(buf))
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

	var out createConversationResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("omi: decode create-conversation response: %w", err)
	}
	if out.Conversation.UUID == "" {
		return "", fmt.Errorf("omi: create-conversation response missing uuid")
	}
	return out.Conversation.UUID, nil
}

func alternateConversationType(convType string) string {
	switch convType {
	case ConversationTypeUnify:
		return ConversationTypeChat
	case ConversationTypeChat:
		return ConversationTypeUnify
	default:
		return ""
	}
}

// DeleteConversation issues DELETE /api/conversations/{uuid}.
// Treats 200/204/404 as success (idempotent). 401 surfaces as *Error so
// callers can distinguish auth failures. Not retried.
func (c *Client) DeleteConversation(ctx context.Context, uuid string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/conversations/"+url.PathEscape(uuid), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// stream=true bypasses Client.do's retry policy. DELETE is intentionally
	// not retried (idempotent on the server but a 500/timeout warrants a
	// single fast warning, not a hidden second request).
	resp, err := c.do(req, true)
	if err != nil {
		return fmt.Errorf("omi: delete conversation: %w", err)
	}
	defer closeutil.Quiet(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	}
	msg := readErrorBody(resp.Body)
	return &Error{Status: resp.StatusCode, Message: string(msg)}
}
