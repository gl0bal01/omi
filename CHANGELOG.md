# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.2] - 2026-05-18

### Fixed
- `omi doctor --live` now routes through `Client.do`, picking up the retry policy, API-KEY auth, debug-HTTP logging, and 64 KiB error-body cap. Previous implementation hand-rolled a one-shot `http.Client` copy that duplicated auth and bypassed every cross-cutting concern.
- Streaming HTTP client (`StreamHTTP`) now uses a hardened `http.Transport` that bounds the pre-stream phases: 30s dial, 10s TLS handshake, 30s response-header (time-to-first-byte). A server that accepts the connection but never writes can no longer hang the client indefinitely when the parent context has no deadline. Body reads after streaming begins continue to rely on the caller's context, so long-running SSE responses are not killed mid-stream.

### Added
- `api.Client.Ping(ctx)` issues a lightweight `GET /models?feature=UNIFY_CHAT_WITH_AI` through the standard `do` path and returns the response status. Used by `omi doctor --live`.
- Unit tests for `internal/redact` covering empty, short, normal, byte-indexed unicode, and middle-leak protection.

### Changed
- `isValidConfigKey` now does an O(1) map lookup instead of scanning `validConfigKeys` each call.
- README install section drops the `curl | sh` placeholder. `omi` is distributed as a single static Go binary via `go install` or the GitHub Releases archives; an install shell wrapper added no value.

## [0.1.1] - 2026-05-16

### Fixed
- `omi consensus`: per-model answers no longer corrupted by mid-stream error text. `consensusAnswer` now writes stderr to `io.Discard` instead of the same buffer that captures the answer fed to the synthesis model.
- Timeout error message now reflects the configured `--timeout` value (or `api.DefaultTimeout`) instead of a hardcoded `60s` literal.
- `DeleteConversation` wraps the conversation UUID with `url.PathEscape` so an unexpected character cannot reshape the request path.

### Changed
- `Transcribe` cascade reduced from three POSTs (`audioUrl` → `audio` → `path`) to two (`audioUrl` → `audio`). One fallback is sufficient for backend-field compatibility, and `/api/features` is non-idempotent so additional retries only enlarge the failure surface.
- Speech-to-text model metadata is now a single `transcribeEntries` registry plus `rawTranscribeModels` for ids without an alias. The legacy slice and map are derived at package init from this source of truth.
- Secret masking is centralized in a new `internal/redact` package. `internal/api` now calls `redact.APIKey` instead of its own `maskSecret`; `config.MaskAPIKey` delegates to the same implementation.
- REPL `bufio.Scanner` initial buffer raised from 1 KiB to 64 KiB so multi-line paste avoids several immediate grow cycles. The 1 MiB ceiling is unchanged.

## [0.1.0] - 2026-05-07

### Added
- Initial release of `omi`, a Go CLI for the 1min.ai REST API.
- Streaming chat via `POST /api/chat-with-ai` with `UNIFY_CHAT_WITH_AI`.
- Vision and document Q&A via the same unified endpoint with `imageList` and `files`.
- Code generation via `POST /api/features` with `CODE_GENERATOR`.
- Audio transcription via `POST /api/features` with `SPEECH_TO_TEXT`.
- Asset upload via `POST /api/assets` (multipart).
- Server-side conversation lifecycle: create on first turn, DELETE on `omi session clear` (idempotent: 404 treated as success, 401 surfaces as hard error).
- Named sessions persisted at `~/.config/omi/sessions.json`.
- Embedded model registry with capability-typed aliases (chat / code-only / vision-only) and per-model defaults (`webSearch`, `numOfSite`, `maxWord`, `conversationType`).
- `~/.config/omi/models.json` override (full-replacement semantics).
- Shell completion for bash, zsh, fish, and powershell.
- REPL when stdin is a TTY; one-shot when stdin is piped.
- Friendly error UX with consistent `omi: <msg>` format and exit codes 0/1/2.
- `omi consensus`, a client-side 3-model panel feature that asks multiple chat models and synthesizes consensus, disagreements, and recommendation. Panel fans out concurrently; wall-clock latency is `max(per-model)` not `sum(per-model)`. Answer order matches the `-m` input order; first error cancels siblings.
- `--api-key` flag (priority: flag → `OMI_API_KEY` env → config file).
- `--timeout` flag for non-streaming HTTP calls (default 60s).
- HTTP retry layer honoring `Retry-After` (RFC 7231 seconds or HTTP-date), capped at 30s, and respecting request-context cancellation.
- Help, quickstart, and README examples for consensus, sessions, and mixed-history usage.
- Repeatable quality gates for fmt, unit tests, race tests, lint, security scanning, vuln scanning, smoke, and full verification.
- CI enforcement for formatting, unit tests, race tests, lint, gosec, govulncheck, builds, and binary smoke.
- Release checklist with versioning, smoke, artifact, and checksum verification steps.
- Automation documentation for stable `--json` outputs.

### Security
- Sanitized streamed content and streamed API error messages to prevent terminal control-sequence injection.
- Session persistence permission tests for 0700 config directories and 0600 session files.
- SIGINT/SIGTERM cancel in-flight HTTP and SSE work; clean cancel exits with status 130 (no "context canceled" noise on stderr).
- Capped `--no-stream` buffered output at 16 MiB with a single stderr truncation warning to bound memory under runaway upstream responses.
- Capped piped-stdin prompt reads at 4 MiB; oversize input fails fast with a usage error.
- Capped non-streaming success bodies (code, transcribe) at 16 MiB and upstream error bodies at 64 KiB to bound log/error surface from a hostile or runaway server.

### Performance
- Cold-start measurement (linux/amd64, Go 1.25): **~6.07ms median** of 10 runs
  of `./bin/omi --version` (`-ldflags "-s -w"`). Well under the <20ms target.
  ```sh
  for i in $(seq 1 10); do /usr/bin/time -f '%e' ./bin/omi --version 2>&1 >/dev/null; done | sort -n | sed -n '5p'
  ```

[0.1.2]: https://github.com/gl0bal01/omi/releases/tag/v0.1.2
[0.1.1]: https://github.com/gl0bal01/omi/releases/tag/v0.1.1
[0.1.0]: https://github.com/gl0bal01/omi/releases/tag/v0.1.0
