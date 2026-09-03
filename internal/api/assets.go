package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gl0bal01/omi/internal/closeutil"
)

// UploadAsset POSTs the file at path to /api/assets as multipart/form-data
// and returns the asset path string assigned by the server.
//
// Field-name compatibility: some backends expect "asset", others "file".
// We try "asset" first, then retry once with "file" only when the first call
// is rejected as a likely validation error.
//
// Accepted response keys (first non-empty wins):
//   - fileContent.path
//   - assetPath
//   - path
//   - result.path
func (c *Client) UploadAsset(ctx context.Context, path string) (string, error) {
	a, err := c.UploadAssetInfo(ctx, path)
	return a.Path, err
}

// Asset is the server-assigned identity of an uploaded file. Path goes in
// attachments.images; UUID (fileContent.uuid, documents only) goes in
// attachments.files.
type Asset struct {
	Path string
	UUID string
}

// UploadAssetInfo is UploadAsset but also returns the document UUID when the
// server provides one.
func (c *Client) UploadAssetInfo(ctx context.Context, path string) (Asset, error) {
	a, err := c.uploadAssetOnce(ctx, path, "asset")
	if err == nil {
		return a, nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) && (apiErr.Status == http.StatusBadRequest || apiErr.Status == http.StatusUnprocessableEntity) {
		return c.uploadAssetOnce(ctx, path, "file")
	}
	return Asset{}, err
}

func (c *Client) uploadAssetOnce(ctx context.Context, path, fieldName string) (Asset, error) {
	f, err := os.Open(path) // #nosec G304 -- uploading an explicit user-supplied path is this command's purpose.
	if err != nil {
		return Asset{}, fmt.Errorf("omi: open %s: %w", path, err)
	}
	defer closeutil.Quiet(f)

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	writeErr := make(chan error, 1)
	go func() {
		defer close(writeErr)

		part, err := mw.CreateFormFile(fieldName, filepath.Base(path))
		if err != nil {
			writeErr <- fmt.Errorf("omi: build multipart: %w", err)
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			writeErr <- fmt.Errorf("omi: read %s: %w", path, err)
			_ = pw.CloseWithError(err)
			return
		}
		if err := mw.Close(); err != nil {
			writeErr <- fmt.Errorf("omi: close multipart: %w", err)
			_ = pw.CloseWithError(err)
			return
		}
		if err := pw.Close(); err != nil {
			writeErr <- fmt.Errorf("omi: close pipe: %w", err)
			return
		}
		writeErr <- nil
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/assets", pr)
	if err != nil {
		return Asset{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	// Multipart body is streaming from disk; retries would require replaying the
	// full body, so this call intentionally bypasses retry logic.
	resp, err := c.do(req, true)
	if err != nil {
		_ = pr.CloseWithError(err)
		select {
		case werr := <-writeErr:
			if werr != nil {
				return Asset{}, werr
			}
		default:
		}
		return Asset{}, err
	}
	defer closeutil.Quiet(resp.Body)
	if werr := <-writeErr; werr != nil {
		return Asset{}, werr
	}

	if resp.StatusCode >= 400 {
		msg := readErrorBody(resp.Body)
		return Asset{}, &Error{Status: resp.StatusCode, Message: string(msg)}
	}

	var raw struct {
		FileContent struct {
			Path string `json:"path"`
			UUID string `json:"uuid"`
		} `json:"fileContent"`
		AssetPath string `json:"assetPath"`
		Path      string `json:"path"`
		Result    struct {
			Path string `json:"path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Asset{}, fmt.Errorf("omi: decode upload response: %w", err)
	}
	switch {
	case raw.FileContent.Path != "":
		return Asset{Path: raw.FileContent.Path, UUID: raw.FileContent.UUID}, nil
	case raw.AssetPath != "":
		return Asset{Path: raw.AssetPath}, nil
	case raw.Path != "":
		return Asset{Path: raw.Path}, nil
	case raw.Result.Path != "":
		return Asset{Path: raw.Result.Path}, nil
	default:
		return Asset{}, fmt.Errorf("omi: upload succeeded but response missing asset path")
	}
}
