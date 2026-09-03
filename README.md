# omi

A single static Go CLI for the [1min.ai](https://app.1min.ai) REST API — with a parallel multi-model **consensus panel**, **Unix-pipe composition**, and a **typed model registry** with per-model defaults.

[![Go Version](https://img.shields.io/badge/go-1.27%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-v0.3.0-brightgreen.svg)](CHANGELOG.md)

```sh
omi consensus -m mini,claude,gemini-pro --synth-model best "Should we ship today?"
```

## Why omi?

- **Multi-model consensus, in parallel.** `omi consensus` fans the same prompt out to an N-model panel concurrently, then a synthesis model produces `Consensus / Disagreements / Recommendation`. Wall-clock = `max(latency)`, not the sum.
- **Composes with the rest of your shell.** TTY-aware stdin/stdout — pipe `git log` or [Fabric](https://github.com/danielmiessler/Fabric) patterns in, pipe streamed output through `jq`, `tee`, or another `omi` call out.
- **Typed model registry, not stringly-typed.** Capability-tagged aliases (chat / code-only / vision-only) with per-model defaults that auto-apply (e.g. `sonar` auto-enables web search). Override via `~/.config/omi/models.json`.
- **Stateful conversations + REPL.** Named sessions persist server-side; an interactive REPL drops in when stdin is a TTY and gracefully handles Ctrl+C cancellation of in-flight streams.
- **Single static binary, ~6 ms cold start.** No Python or Node runtime, no CGO. Production-safe defaults: `Retry-After` honored, resource caps, terminal-injection sanitizer.

## Features

- **Streaming chat** over Server-Sent Events for sub-second first-token latency.
- **Vision and document Q&A** via the unified `chat-with-ai` endpoint with image or PDF/text/docx attachments.
- **Code generation** with a dedicated `omi code` subcommand and code-capable model aliases.
- **Consensus mode** via `omi consensus`, an `omi` client-side feature that asks multiple chat models and synthesizes their answers.
- **Audio transcription** via `omi transcribe`.
- **Named, persistent sessions** with server-side conversation lifecycle (create + delete) and an interactive REPL when stdin is a TTY.

## Install

### `go install`

```sh
go install github.com/gl0bal01/omi/cmd/omi@latest
```

### Manual download

Grab the archive for your platform from the [GitHub Releases](https://github.com/gl0bal01/omi/releases) page.

| OS      | Architecture | Archive                              |
|---------|--------------|--------------------------------------|
| Linux   | amd64        | `omi-<version>-linux-amd64.tar.gz`   |
| Linux   | arm64        | `omi-<version>-linux-arm64.tar.gz`   |
| macOS   | amd64        | `omi-<version>-darwin-amd64.tar.gz`  |
| macOS   | arm64        | `omi-<version>-darwin-arm64.tar.gz`  |
| Windows | amd64        | `omi-<version>-windows-amd64.zip`    |

Extract the archive and place `omi` (or `omi.exe`) somewhere on your `PATH`. Verify with `omi --version`.

## Quickstart

```sh
omi config set api_key sk-...
omi "explain Go interfaces in one paragraph"
omi -f photo.jpg "what's in this image?"
omi consensus "Should this CLI default to streaming responses?"
omi -s research "Remember this project context"
omi -s research "Use the context from earlier"
omi --mixed "Compare this with prior context"
omi -s research --mixed "Continue with mixed history"
omi session list
omi session clear research
```

## Command reference

| Command                      | Purpose                                                       |
|------------------------------|---------------------------------------------------------------|
| `omi [prompt]`               | Chat (streaming SSE). Empty prompt + TTY enters REPL.         |
| `omi -f <file> [prompt]`     | Vision (image) or document Q&A (pdf/txt/docx) via chat.       |
| `omi code <prompt>`          | Code generation via `CODE_GENERATOR`.                         |
| `omi consensus <prompt>`     | Ask a 3-model panel and synthesize a consensus locally.        |
| `omi transcribe <audio>`     | Speech-to-text via `SPEECH_TO_TEXT`.                          |
| `omi transcribe models`      | List transcribe aliases and raw speech model IDs.             |
| `omi quickstart`             | Print recommended presets and examples by use case.           |
| `omi upload <file>`          | Upload an asset and print the returned asset path.            |
| `omi session list`           | List saved sessions (name → UUID).                            |
| `omi session clear <name>`   | Delete a single session server-side and locally.              |
| `omi session clear --all`    | Delete every saved session (prompts unless `-y`).             |
| `omi config set <k> <v>`     | Persist a config value to `~/.config/omi/config.json`.        |
| `omi config get <k>`         | Print a config value (`api_key` masked unless `--reveal`).    |
| `omi config list`            | Print every config key (api_key masked).                      |
| `omi models [chat\|code\|vision\|all]` | List model aliases with CAPS/DEFAULTS/NOTES metadata. |
| `omi models explain <id>`    | Show one model’s capabilities, defaults, and related aliases. |
| `omi doctor`                 | Validate config, effective models, and endpoint compatibility. |
| `omi completion <shell>`     | Emit a shell completion script (bash, zsh, fish, powershell). |
| `omi --version`              | Print the binary version and exit.                            |
| `omi --help`                 | Print top-level help.                                         |

## Flags (root chat command)

| Flag                   | Description                                                 |
|------------------------|-------------------------------------------------------------|
| `-m, --model <id>`     | Chat model alias (or raw API id with stderr warning).       |
| `--task <name>`        | Task preset (`chat`, `code`, `vision`, `research`) for auto model choice. |
| `-s, --session <name>` | Use a named, server-persisted conversation.                 |
| `-w, --web`            | Enable web search.                                          |
| `--no-web`             | Force-disable web search (suppresses per-model defaults).   |
| `-M, --mixed`          | Enable mixed-history context (compat keys: `isMixed`, `historyMixed`, `history_mixed`). |
| `--no-stream`          | Buffer the response and print at the end.                   |
| `-n, --num-sites <N>`  | Cap `numOfSite` for web search.                             |
| `--max-words <N>`      | Cap `maxWord` on the response.                              |
| `-f, --file <path>`    | Attach an image or document to the chat turn.               |
| `--api-key <key>`      | API key (overrides `OMI_API_KEY` env and config file).      |
| `--timeout <dur>`      | Per-request timeout for non-streaming calls (default `60s`). |
| `--verbose`            | Print effective endpoint/model/settings before request.      |
| `--explain-defaults`   | Explain why defaults were selected (flags vs model defaults).|
| `--debug-http`         | Print redacted HTTP request/response summaries.              |

## Environment variables

| Variable          | Effect                                                                 |
|-------------------|------------------------------------------------------------------------|
| `OMI_API_KEY`     | Overrides `config.api_key`. Required if `api_key` is not configured.   |
| `OMI_MODEL`       | Default chat model. Overridden by `-m/--model`.                        |
| `OMI_CODE_MODEL`  | Default model for `omi code`. Overridden by `--code-model`.            |
| `XDG_CONFIG_HOME` | Base for config directory (`$XDG_CONFIG_HOME/omi/...`). Falls back to `~/.config`. |

## Models

`omi` ships with an embedded registry of capability-typed aliases.

`omi models` (no arg) shows chat-capable aliases by default. Use:
- `omi models code`
- `omi models vision`
- `omi models all`
- `omi models --json`
- `omi models explain <alias-or-apiId>`

Output columns are:
- `ALIAS` (shortcut)
- `APIID` (raw upstream id)
- `CAPS` (capability tags)
- `DEFAULTS` (auto-applied per-model defaults)
- `NOTES` (quick usage hints)

### Chat aliases (default `mini` → `gpt-4o-mini`)

| Alias                  | API id                            | Notes                              |
|------------------------|-----------------------------------|------------------------------------|
| `best`                 | `gpt-5.5`                         | Best general chat quality.         |
| `mini`                 | `gpt-4o-mini`                     | Default chat model (cost/speed).   |
| `gpt55-pro`            | `gpt-5.5-pro`                     | High-end reasoning.                |
| `gpt56-sol`            | `gpt-5.6-sol`                     | GPT-5.6 family (also `gpt56-luna`, `gpt56-terra`). |
| `gpt54`                | `gpt-5.4`                         | Strong general reasoning.          |
| `o3`                   | `o3`                              | Deep reasoning.                    |
| `claude`               | `claude-sonnet-5`                 | Great long-form synthesis.         |
| `claude-opus`          | `claude-opus-5`                   | Deep analysis.                     |
| `claude-fable`         | `claude-fable-5`                  | Top-tier Anthropic model.          |
| `gemini-pro`           | `gemini-3.1-pro-preview`          | Long-context + strong analysis.    |
| `gemini35-flash`       | `gemini-3.5-flash`                | Fast Gemini.                       |
| `deepseek`             | `deepseek-chat`                   | Fast, low-cost general chat.       |
| `deepseek-v4`          | `deepseek-v4-pro`                 |                                    |
| `grok`                 | `grok-4-0709`                     |                                    |
| `grok46`               | `grok-4.6`                        | Latest Grok.                       |
| `glm`                  | `glm-5.3`                         |                                    |
| `sonar`                | `sonar-pro`                       | Auto web search, `numOfSite=5`.    |
| `deep-research`        | `sonar-deep-research`             | Auto web search, `numOfSite=5`.    |
| `qwen-max`             | `qwen3-max`                       |                                    |
| `qwen37-max`           | `qwen3.7-max`                     |                                    |

### Code-only aliases

| Alias            | API id                | Notes                                |
|------------------|-----------------------|--------------------------------------|
| `codex`          | `gpt-5.3-codex`       | Default code model.                  |
| `qwen-code`      | `qwen3.7-plus`        | Strong alt for code-heavy prompts.   |
| `qwen-code-fast` | `qwen3.7-flash`       | Fast code-focused responses.         |
| `grok-code`      | `grok-4.6`            |                                      |

### Vision-only aliases

| Alias           | API id            |
|-----------------|-------------------|
| `qwen-vl`       | `qwen3-vl-plus`   |
| `qwen-vl-fast`  | `qwen3-vl-flash`  |
| `pixtral`       | `pixtral-12b`     |

A model alias not found in the registry is passed through to the API verbatim and a stderr warning is emitted. Known API IDs from the embedded registry are accepted without warning.

Refresh check against live 1min.ai catalogs:

```sh
scripts/sync-models.sh --strict
```

## `models.json` override

The override file at `~/.config/omi/models.json` (or `$XDG_CONFIG_HOME/omi/models.json`) **fully replaces** the embedded set. There is no merge. To add a single alias, copy the embedded JSON, append your entry, and save the result. On parse error, `omi` logs a warning to stderr and falls back to the embedded registry.

## Sessions and ephemeral REPL

Unnamed REPL sessions are ephemeral. The conversation persists on the 1min.ai server but is not tracked locally. Pass `-s <name>` to make a session persistent and reusable.

When the REPL exits (`exit`, `quit`, Ctrl+D) without `-s`, the auto-created server-side conversation continues to exist on the 1min.ai backend but is dropped from local tracking.

Session examples:

```sh
omi -s research "Remember this project context"
omi -s research "Use the context from earlier"
omi session list
omi session clear research
```

Mixed-history examples:

```sh
omi --mixed "Compare this with prior context"
omi -s research --mixed "Continue with mixed history"
```

## Consensus

`omi consensus` is an `omi` feature, not a separate 1min.ai endpoint. The CLI sends the same prompt to a 3-model panel of normal chat models, then sends those answers to a synthesis model to produce a consensus report.

Default panel:

```sh
omi consensus "Should we use SQLite or Postgres for this local CLI?"
```

Custom panel and synthesis model:

```sh
omi consensus -m mini,claude,gemini-pro --synth-model best "Should this release go out today?"
```

The output includes each model's answer followed by a synthesis with `Consensus`, `Disagreements`, and `Recommendation` sections.

## `--no-web` and per-model defaults

Some models (e.g., `sonar`, `sonar-reasoning`) have `webSearch=true` as a per-model default. `omi` auto-enables web search for these and emits a stderr notice. The explicit `--no-web` flag suppresses this. The explicit `-w/--web` flag forces web search on (no double notice). User-provided `-n/--num-sites` and `--max-words` always override per-model defaults.

Precedence for the final `webSearch` value:

1. `--no-web` → `false`, no notice.
2. `-w/--web` → `true`, no notice.
3. Otherwise → use `defaults.webSearch` from the registry.

## Composition with Unix pipes

`omi` reads piped stdin as a single-turn prompt when stdout is a TTY but
stdin is not, and writes streamed model output to stdout. That makes it
composable with any stdin/stdout tool.

Pipe upstream → `omi`:

```sh
cat notes.md | omi "summarize this in five bullet points"
git log --since='1 week' --oneline | omi -m claude "release notes draft"
```

Pipe `omi` → downstream:

```sh
omi -m best "write a structured incident report for $INCIDENT" \
  | tee report.md \
  | grep -i 'action item'
```

Compose with [Fabric](https://github.com/danielmiessler/Fabric) patterns —
Fabric brings curated prompt patterns (`extract_wisdom`, `summarize`,
`improve_writing`, …), `omi` brings the 1min.ai model panel. Either tool
can drive, the other refines:

```sh
# Fabric extracts → omi rewrites for a specific audience
cat article.md | fabric --pattern extract_wisdom \
  | omi -m claude "rewrite this for a non-technical reader"

# omi drafts → Fabric polishes the prose
omi --no-stream "draft a postmortem for $INCIDENT" \
  | fabric --pattern improve_writing
```

Use `--no-stream` when piping into a tool that buffers stdin or expects a
single complete document.

## Resource limits and cancellation

`omi` bounds memory and surface area against hostile or runaway upstream
responses:

| Limit                              | Value      | Behavior on overflow                                  |
|------------------------------------|------------|--------------------------------------------------------|
| `--no-stream` buffered output      | 16 MiB     | Truncated; one `omi: warning:` line on stderr.         |
| Piped-stdin prompt                 | 4 MiB      | Fails fast with a usage error.                         |
| Non-streaming success body         | 16 MiB     | Read via `io.LimitReader`; tail bytes silently dropped.|
| Upstream error body                | 64 KiB     | Read via `io.LimitReader`; truncated for `*api.Error`. |
| `Retry-After` honored on 429       | up to 30 s | Capped to prevent server-side stall attacks.            |

`omi` installs a `signal.NotifyContext` for `SIGINT`/`SIGTERM` so Ctrl+C
cancels in-flight HTTP and SSE work cleanly. Clean cancel exits **130**.

## Automation And Quality Gates

Stable JSON output is available for automation on:

- `omi models --json`: object with `filter`, `count`, and `entries`; each entry has `alias`, `apiId`, `caps`, `defaults`, and `notes`.
- `omi models explain <id> --json`: object with `query`, `alias`, `apiId`, `caps`, `defaults`, `notes`, and `relatedAliases`.
- `omi transcribe models --json`: object with `default`, `aliases`, `models`, `endpoint`, and `type`.

Local quality gates are repeatable through `make`:

```sh
make fmt-check
make test
make race
make lint
make security
make vuln
make build
make verify
```

`make smoke` runs the live smoke suite and requires `OMI_API_KEY`; set `OMI_SMOKE_AUDIO` to include transcription of a real audio fixture. `omi doctor` is the quick local compatibility audit, and `omi doctor --live` adds a lightweight authenticated model-catalog request.

Release steps and artifact paths are documented in `docs/release-checklist.md`.

## Shell completion

Generate completion scripts with `omi completion <shell>`.

### Load directly into the current shell

No install, lasts until the shell exits.

```sh
# bash
source <(omi completion bash)

# zsh
source <(omi completion zsh)
compdef _omi omi   # only needed the first time per shell

# fish
omi completion fish | source

# powershell
omi completion powershell | Out-String | Invoke-Expression
```

### Persist across sessions

```sh
# bash (system-wide)
omi completion bash | sudo tee /etc/bash_completion.d/omi >/dev/null
# bash (user-local)
omi completion bash > ~/.local/share/bash-completion/completions/omi

# zsh — drop into a directory on $fpath, then `autoload -Uz compinit && compinit`
omi completion zsh > "${fpath[1]}/_omi"

# fish
omi completion fish > ~/.config/fish/completions/omi.fish

# powershell — append the load line to your $PROFILE
omi completion powershell | Out-String | Invoke-Expression
```

Completion covers subcommands, the `-m/--model` flag (filtered by capability for the active command), and the `-s/--session` flag (saved session names). Verify with `omi <TAB><TAB>`.

## Architecture

```
cmd/omi/                      // entrypoint; reads -ldflags `main.version`
internal/api/                 // HTTP client, chat (SSE), assets, code, speech, conversations
internal/cmd/                 // cobra commands (root, code, transcribe, upload, session, config, models, completion, repl)
internal/config/              // ~/.config/omi/config.json load + save (0700 dir, 0600 file)
internal/models/              // embedded registry + override loader
internal/session/             // ~/.config/omi/sessions.json
internal/stream/              // 80-LoC SSE parser
```

The CLI uses Cobra, stdlib `net/http`, and a hand-rolled SSE parser. No CGO. No vendored deps beyond `cobra` and `golang.org/x/term`. All chat traffic — including vision and document Q&A — flows through `POST /api/chat-with-ai` with the `UNIFY_CHAT_WITH_AI` type. `CODE_GENERATOR` and `SPEECH_TO_TEXT` use `POST /api/features`.

## License

[MIT](LICENSE)
