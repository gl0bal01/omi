# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`omi` is a single static Go CLI (Cobra + stdlib `net/http`, no CGO) for the 1min.ai REST API. Module: `github.com/gl0bal01/omi`, Go 1.27. Only deps: `spf13/cobra`, `golang.org/x/term`. Do not add dependencies without strong reason.

## Commands

```sh
make build            # → bin/omi (ldflags set main.version from VERSION)
make test             # go test ./...
make race             # go test -race ./...
go test ./internal/cmd -run TestSession    # single package / test
go test ./internal/api -run 'TestChat.*' -v
make fmt              # gofmt -w
make lint             # fmt-check + golangci-lint v2.13.2 (errcheck, govet, ineffassign, staticcheck, unused, goimports)
make quality          # fmt-check test race lint
make verify           # quality + gosec + govulncheck + build — release gate
make smoke            # live API suite; needs OMI_API_KEY (OMI_SMOKE_AUDIO optional)
scripts/sync-models.sh --strict   # diff internal/models/models.json vs live 1min.ai catalog (needs jq, network)
```

CI (`.github/workflows/ci.yml`) runs gofmt, test, race on linux/macos/windows; lint/gosec/govulncheck on linux only. Pinned Go 1.27.1. Windows must stay green — avoid Unix-only assumptions in tests.

Release: see `docs/release-checklist.md` (`VERSION=x.y.z make verify`, tag `vx.y.z`, GoReleaser via `release.yml`). Update `CHANGELOG.md` and `cmd/omi/main.go` default `version` for releases.

## Architecture

```
cmd/omi/main.go     signal.NotifyContext (SIGINT/SIGTERM) → cmd.ExecuteContext; exit codes: 130 clean cancel, 2 *UsageError, 1 *RuntimeError/other
internal/cmd/       cobra commands; root.go = chat path, newRootCmd wires all subcommands
internal/api/       Client (auth header API-KEY, retries, debug logging) + one file per endpoint
internal/models/    embedded models.json registry (go:embed) + ~/.config/omi/models.json override
internal/stream/    hand-rolled SSE parser → chan Event
internal/config/    config.json (0700 dir / 0600 file, atomic write)
internal/session/   sessions.json name → conversation UUID
internal/redact/    API-key masking for logs
```

**Request flow for chat** (`internal/cmd/root.go` `runChat`): empty prompt → `dispatchEmpty` (TTY = REPL, piped stdin = single-turn) → `loadAPIKey` → `resolveModelInputWithTask` (flag > `OMI_MODEL` > config > `--task` preset > `gpt-4o-mini`) → `models.Resolve` (capability check: chat path rejects code-only/vision-only aliases; `-f image` rejects only code-only aliases since chat models are multimodal) → `resolveSettings` (webSearch precedence: `--no-web` > `--web` > model default with stderr notice) → session lookup/create → `buildChatRequest` → optional `UploadAsset` → `client.Chat` (SSE) → `consumeStream`.

**Endpoints.** All chat incl. vision/docs: `POST /api/chat-with-ai?isStreaming=true` with type `UNIFY_CHAT_WITH_AI`. `CODE_GENERATOR` and `SPEECH_TO_TEXT`: `POST /api/features` (non-idempotent — callers do their own single shape-fallback retry, never generic retries). Assets: `POST /api/assets` multipart; response `fileContent.path` goes in `attachments.images`, `fileContent.uuid` in `attachments.files` (documents are resolved by UUID, not path). Server rejects `text/markdown` uploads. Conversations: `POST/DELETE /api/conversations`.

**Retry policy** (`api.Client.do`): non-streaming only, on connection errors / 5xx, `Backoffs` = 100ms, 300ms, `Retry-After` honored up to 30s. Streaming requests are never retried. 4xx never retried.

**Consensus** (`internal/cmd/consensus.go`) is client-side: fan-out to N models concurrently via non-streaming chat, then a synthesis-model call. Not an upstream feature.

**Model registry.** Aliases carry `Caps` bitmask (chat/code/vision) and `ModelDefaults` (webSearch, numOfSite, maxWord, conversationType). Override file **fully replaces** the embedded set, no merge; parse failure falls back with stderr warning. Unknown alias passes through as raw ID with a warning; known raw API IDs pass silently. `internal/cmd/models.go` lists/explains with `--json`. When adding a model: edit `internal/models/models.json`, update README tables, run `scripts/sync-models.sh --strict`.

## Conventions

- Errors returned to the user: `*UsageError` (exit 2) vs `*RuntimeError` (exit 1), messages prefixed `omi: `. Wrap API errors with `TranslateAPIError`. Never print raw upstream text — pass through `SanitizeForTerminal` / `sanitizeForTerminalText` (terminal-injection guard).
- Diagnostics (`omi: note:`, `omi: hint:`, `omi: warning:`, `omi: effective:`, `omi: debug:`) go to stderr; only model output goes to stdout so pipes stay clean.
- Resource caps are deliberate: 16 MiB no-stream buffer and response body, 4 MiB piped stdin, 64 KiB error body, 1 MiB config/session files, 4 MiB registry. Keep `io.LimitReader` on any new body read.
- Config/session files: atomic write with 0600 perms via `atomicWrite0600`; `XDG_CONFIG_HOME` overrides `~/.config`. Tests set `XDG_CONFIG_HOME` to a temp dir and use `httptest.NewServer` with `api.NewClient(key, srv.URL, 0)`.
- `--api-key` flag is documented as insecure; prefer `OMI_API_KEY` or `omi config set api_key -` (reads secret from stdin).
- Stable `--json` output shapes (models, models explain, transcribe models) are a compatibility contract; changing fields is a breaking change.
