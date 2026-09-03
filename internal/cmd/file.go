package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
)

type fileKind int

const (
	fileImage fileKind = iota
	fileDoc
	fileAudio
)

// detectFile classifies a path by its extension into image, doc, or audio.
// Unknown extensions return an error the caller can surface verbatim.
func detectFile(path string) (fileKind, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return fileImage, nil
	case ".pdf", ".txt", ".docx":
		return fileDoc, nil
	case ".mp3", ".wav", ".m4a", ".ogg", ".flac", ".webm":
		return fileAudio, nil
	default:
		return 0, fmt.Errorf("omi: unsupported file type: %s (expected image, pdf, txt, docx, audio; markdown is rejected upstream, rename to .txt)", ext)
	}
}
