// SPEECH_TO_TEXT remains on /api/features per official 1min.ai docs (not in deprecation list).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gl0bal01/omi/internal/closeutil"
)

const featureTypeSpeechToText = "SPEECH_TO_TEXT"

// speechRequest mirrors /api/features. Two upstream field-name shapes are
// supported: "audioUrl" (current) and "audio" (legacy fallback).
type speechRequest struct {
	Type         string             `json:"type"`
	Model        string             `json:"model"`
	PromptObject speechPromptObject `json:"promptObject"`
}

type speechPromptObject struct {
	AudioURL       string `json:"audioUrl,omitempty"`
	Audio          string `json:"audio,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

// Transcribe POSTs SPEECH_TO_TEXT to /api/features for an already-uploaded
// audio asset. One retry uses the legacy "audio" field on 400/422. No
// further retries — /api/features is non-idempotent.
func (c *Client) Transcribe(ctx context.Context, assetPath, model string) (string, error) {
	text, err := c.transcribeOnce(ctx, speechRequest{
		Type:  featureTypeSpeechToText,
		Model: model,
		PromptObject: speechPromptObject{
			AudioURL:       assetPath,
			ResponseFormat: "text",
		},
	})
	if err == nil {
		return text, nil
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || (apiErr.Status != http.StatusBadRequest && apiErr.Status != http.StatusUnprocessableEntity) {
		return "", err
	}
	return c.transcribeOnce(ctx, speechRequest{
		Type:  featureTypeSpeechToText,
		Model: model,
		PromptObject: speechPromptObject{
			Audio:          assetPath,
			ResponseFormat: "text",
		},
	})
}

func (c *Client) transcribeOnce(ctx context.Context, body speechRequest) (string, error) {
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
		return "", fmt.Errorf("omi: transcribe response missing result text")
	}
	return text, nil
}

// extractFeatureResult walks /api/features response shapes for the result
// text. Tries aiRecord.aiRecordDetail.resultObject (string or []string),
// then result.response, then a top-level "result" string.
func extractFeatureResult(raw []byte) string {
	var shape struct {
		AIRecord struct {
			AIRecordDetail struct {
				ResultObject json.RawMessage `json:"resultObject"`
			} `json:"aiRecordDetail"`
		} `json:"aiRecord"`
		Result struct {
			Response string `json:"response"`
		} `json:"result"`
		TopResult string `json:"result_text"`
	}
	if err := json.Unmarshal(raw, &shape); err == nil {
		if len(shape.AIRecord.AIRecordDetail.ResultObject) > 0 {
			var s string
			if err := json.Unmarshal(shape.AIRecord.AIRecordDetail.ResultObject, &s); err == nil && s != "" {
				return s
			}
			var arr []string
			if err := json.Unmarshal(shape.AIRecord.AIRecordDetail.ResultObject, &arr); err == nil && len(arr) > 0 {
				return arr[0]
			}
		}
		if shape.Result.Response != "" {
			return shape.Result.Response
		}
		if shape.TopResult != "" {
			return shape.TopResult
		}
	}
	return ""
}
