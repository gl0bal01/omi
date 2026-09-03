# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Document Q&A (`-f file.pdf|txt|docx`) now passes the upload's `fileContent.uuid` in `attachments.files` instead of the asset path. Upstream resolves documents by UUID; with the path the model never saw the file content.
- Image attachments now work with any chat alias (`-m claude -f photo.png`). The registry previously rejected every non-vision-only alias even though every chat model on `chat-with-ai` is multimodal; only code-only aliases are rejected now.
- `omi transcribe` no longer inherits `OMI_MODEL` / `config.model`. Those are chat models; with either set (e.g. `claude`) every transcription failed upstream. Speech model now comes only from `transcribe -m` or the `qwen3-asr-flash` default. `omi doctor` reports the same.
- `.md` attachments are rejected client-side with a hint to rename to `.txt`; the upload endpoint returns `UNSUPPORTED_FILE_TYPE` for `text/markdown`.

### Changed
- Model registry synced against the live 1min.ai catalog (2026-09-03). Repointed: `best` → `gpt-5.5`, `claude` → `claude-sonnet-5`, `claude-opus` → `claude-opus-5`, `codex` → `gpt-5.3-codex`, `qwen-code` → `qwen3.7-plus`, `qwen-code-fast` → `qwen3.7-flash`, `grok-code` → `grok-4.6`. Added: `gpt55`, `gpt55-pro`, `gpt56-luna|sol|terra`, `claude-fable`, `gemini35-flash`, `deepseek-v4`, `deepseek-v4-flash`, `grok43|45|46`, `glm`, `glm5`, `qwen36-plus|flash`, `qwen37-max|plus|flash`. Removed (no longer served): `codex-mini`, `gptoss`, `gptoss20`, `o4-mini`, `o4-mini-dr`, `o3-deep-research`, and every Claude 4.x alias (`claude4`, `claude45`, `claude46`, `claude-haiku`, `claude-opus4`, `claude-opus41`, `claude-opus45`, `claude-opus46`; `claude-opus-4-7/4-8` are listed in the catalog but rejected by the server).
- `--task code` preset and `omi code` default now use `gpt-5.3-codex`.
- Toolchain and deps bumped: Go 1.27.1 (go.mod + CI), `golang.org/x/term` 0.45.0, `golang.org/x/sys` 0.47.0, `spf13/pflag` 1.0.10, golangci-lint 2.13.2, gosec 2.29.0, govulncheck 1.7.0. Makefile `gosec`/`govulncheck` pinned instead of `@latest`.
- `Makefile VERSION`, `main.go` default version, and README status badge aligned to 0.2.0 (they still said 0.1.0 after the v0.2.0 tag).
- `claude` aliases no longer carry `conversationType: code`, so plain chat with Claude no longer prints the "works best with omi code" hint.

## [0.2.0] - 2026-05-30

### Security
- HTTP clients now refuse cross-host and non-HTTPS redirects via `CheckRedirect` and strip the `API-KEY` header before refusing. Go's stdlib only drops a fixed allowlist of sensitive headers (`Authorization`, `Cookie`, ...) on cross-domain redirects; the custom `API-KEY` header is not on that list and would otherwise be re-attached when following a 3xx to an attacker-chosen host, leaking the credential.
- SSE streaming is now bounded: 64 MiB total per response (`LimitReader`), 1 MiB per line (`bufio.Scanner` buffer cap), and 16 MiB per event. A hostile or compromised upstream can no longer exhaust client memory with an unterminated or oversized event.
- Both HTTP transports pin an explicit TLS floor (`MinVersion: TLS 1.2`). Certificate verification remains at the secure default (`InsecureSkipVerify` is never set).
- Config, session, and `models.json` reads are size-capped (1–4 MiB) before JSON unmarshal to bound memory on a malformed or hostile file.
- `config.json` and `sessions.json` are written atomically (temp file + rename, `0600`, `O_EXCL`, no symlink-follow), eliminating the truncate-then-chmod window, torn writes on crash, and writes through a pre-planted symlink.
- API-key masking threshold raised so short keys collapse to `***` instead of revealing the entire value; the long-key display format (`sk-...XXXX`) is unchanged.
- The asset path returned by `omi upload` is passed through terminal sanitization before printing, closing the one upstream-string-to-stdout path that previously skipped it.
- CI/CD supply chain hardened: GitHub Actions are pinned to commit SHAs (checkout, setup-go, goreleaser); `gosec` and `govulncheck` are pinned to fixed versions instead of `@latest`; CI is granted least-privilege `permissions: contents: read`.
- Bumped `golang.org/x/sys` 0.43.0 → 0.45.0 to clear advisory GO-2026-5024.

### Added
- `omi config set api_key -` reads the API key from stdin (or a no-echo prompt on a terminal) so the secret never appears in the process list or shell history.
- A stderr note is emitted when a `models.json` override is active, surfacing alias remapping that was previously silent.
- Regression tests: cross-host redirect refusal (proves the API key does not leak), oversized SSE line/event rejection, and updated masking thresholds.

### Changed
- `--api-key` flag help now marks the flag as insecure (visible in process list and shell history) and points to `OMI_API_KEY` / `omi config set api_key -`.
- `scripts/smoke.sh` enables `pipefail` (`-e` intentionally omitted so the assertion harness keeps tallying).

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

[0.2.0]: https://github.com/gl0bal01/omi/releases/tag/v0.2.0
[0.1.2]: https://github.com/gl0bal01/omi/releases/tag/v0.1.2
[0.1.1]: https://github.com/gl0bal01/omi/releases/tag/v0.1.1
[0.1.0]: https://github.com/gl0bal01/omi/releases/tag/v0.1.0
