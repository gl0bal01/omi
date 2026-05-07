#!/usr/bin/env bash
# smoke.sh — live integration smoke for omi against the 1min.ai API.
#
# Exercises every command path, including the Phase-0 best-guess shapes
# (asset multipart field, audioUrl, imageList/files) that aren't proven
# until a real server responds. Failures here are the signal to fix the
# matching internal/api/*.go file, not the smoke script.
#
# Requirements:
#   - OMI_API_KEY  — real 1min.ai key. REQUIRED.
#   - omi binary   — on PATH or via BIN=/path/to/omi.
#   - OMI_SMOKE_AUDIO — optional path to an audio file (transcribe is
#     skipped if unset; otherwise must be readable .mp3/.wav/.m4a/etc).
#
# Side effects: uses a tmp XDG_CONFIG_HOME so your real ~/.config/omi
# is untouched. Sessions created during the run are cleaned up on exit.
#
# Exit: 0 = all green, 1 = at least one failure, 2 = bad invocation.

set -u

# --- Setup ---------------------------------------------------------------

BIN=${BIN:-omi}
if ! command -v "$BIN" >/dev/null 2>&1 && [ ! -x "$BIN" ]; then
    printf 'smoke: omi binary not found (set BIN=/path/to/omi)\n' >&2
    exit 2
fi
if [ -z "${OMI_API_KEY:-}" ]; then
    printf 'smoke: OMI_API_KEY is required (export it before running).\n' >&2
    exit 2
fi

WORKDIR=$(mktemp -d -t omi-smoke.XXXXXX)
export XDG_CONFIG_HOME="$WORKDIR/cfg"
mkdir -p "$XDG_CONFIG_HOME/omi"
chmod 700 "$XDG_CONFIG_HOME/omi"
unset OMI_MODEL OMI_CODE_MODEL || true

SESS_PREFIX="omi-smoke-$$"

cleanup() {
    "$BIN" session list 2>/dev/null \
        | awk -v p="$SESS_PREFIX" '$1 ~ "^"p {print $1}' \
        | while read -r s; do
            "$BIN" session clear "$s" >/dev/null 2>&1 || true
          done
    rm -rf "$WORKDIR"
}
trap cleanup EXIT

PASS=0; FAIL=0; SKIP=0

# Color helpers (only when stdout is a TTY).
if [ -t 1 ]; then
    C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YEL=$'\033[33m'; C_RST=$'\033[0m'
else
    C_GREEN=''; C_RED=''; C_YEL=''; C_RST=''
fi

log_pass() { printf '%sPASS%s %s\n' "$C_GREEN" "$C_RST" "$1"; PASS=$((PASS+1)); }
log_fail() {
    printf '%sFAIL%s %s\n' "$C_RED" "$C_RST" "$1"
    [ -n "${2:-}" ] && printf '     %s\n' "$2"
    FAIL=$((FAIL+1))
}
log_skip() { printf '%sSKIP%s %s — %s\n' "$C_YEL" "$C_RST" "$1" "$2"; SKIP=$((SKIP+1)); }
section()  { printf '\n=== %s ===\n' "$1"; }

# run <desc> <want_exit> -- <cmd...>
run() {
    local desc=$1 want_ec=$2; shift 2
    local ec=0
    "$@" >"$WORKDIR/last_out" 2>"$WORKDIR/last_err" || ec=$?
    if [ "$ec" -ne "$want_ec" ]; then
        log_fail "$desc" "exit=$ec want=$want_ec"
        head -3 "$WORKDIR/last_err" 2>/dev/null | sed 's/^/     stderr: /'
        head -3 "$WORKDIR/last_out" 2>/dev/null | sed 's/^/     stdout: /'
        return 1
    fi
    log_pass "$desc"
    return 0
}

expect_stdout() {
    if grep -qE "$2" "$WORKDIR/last_out"; then
        log_pass "$1"
    else
        log_fail "$1" "stdout missing pattern: $2"
    fi
}
expect_stderr() {
    if grep -qE "$2" "$WORKDIR/last_err"; then
        log_pass "$1"
    else
        log_fail "$1" "stderr missing pattern: $2"
    fi
}
expect_no_stderr() {
    if grep -qE "$2" "$WORKDIR/last_err"; then
        log_fail "$1" "stderr should NOT match: $2"
    else
        log_pass "$1"
    fi
}

# --- Fixtures ------------------------------------------------------------

PNG="$WORKDIR/pixel.png"
TXT="$WORKDIR/sample.txt"
PDF="$WORKDIR/sample.pdf"
BAD="$WORKDIR/bad.xyz"

# 1x1 transparent PNG
printf 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=' \
    | base64 -d > "$PNG"

printf 'omi smoke fixture: a tiny doc about distributed databases.\n' > "$TXT"

cat > "$PDF" <<'EOF'
%PDF-1.1
1 0 obj <</Type /Catalog /Pages 2 0 R>> endobj
2 0 obj <</Type /Pages /Kids [3 0 R] /Count 1>> endobj
3 0 obj <</Type /Page /Parent 2 0 R /Resources <<>> /MediaBox [0 0 100 100]>> endobj
xref
0 4
0000000000 65535 f
0000000009 00000 n
0000000052 00000 n
0000000099 00000 n
trailer <</Size 4 /Root 1 0 R>>
startxref
160
%%EOF
EOF

printf 'garbage\n' > "$BAD"

# --- Tests: build / sanity ----------------------------------------------

section "build / sanity"
run "binary --version" 0 "$BIN" --version
run "binary --help"    0 "$BIN" --help

# --- Tests: config (offline, isolated XDG) ------------------------------

section "config command (offline)"
run "config set num_sites 4" 0 "$BIN" config set num_sites 4
run "config get num_sites"   0 "$BIN" config get num_sites
expect_stdout "num_sites round-trip" '^4$'
run "config set model mini"  0 "$BIN" config set model mini
run "config get model"       0 "$BIN" config get model
expect_stdout "model round-trip" '^mini$'
run "config set api_key sk-test1234567890" 0 "$BIN" config set api_key sk-test1234567890
run "config get api_key (masked)" 0 "$BIN" config get api_key
expect_stdout "api_key masked" '^sk-\.\.\.7890$'
run "config get api_key --reveal" 0 "$BIN" config get api_key --reveal
expect_stdout "api_key revealed" '^sk-test1234567890$'
run "config list" 0 "$BIN" config list
expect_stdout "config list shows api_key masked" 'api_key = sk-\.\.\.7890'

run "config set unknown key (rejected)" 2 "$BIN" config set bogus_key x
expect_stderr "unknown key error" 'unknown config key'

# Strip the test api_key so live calls fall back to OMI_API_KEY env.
rm -f "$XDG_CONFIG_HOME/omi/config.json"

# --- Tests: models (offline) --------------------------------------------

section "models meta (offline)"
run "models (chat)" 0 "$BIN" models
expect_stdout "mini in chat" 'mini'
run "models code"   0 "$BIN" models code
expect_stdout "codex in code" 'codex'
run "models vision" 0 "$BIN" models vision
expect_stdout "pixtral in vision" 'pixtral'
run "models bad filter (rejected)" 2 "$BIN" models bogus

# --- Tests: completion (offline) ----------------------------------------

section "completion scripts (offline)"
run "completion zsh"        0 "$BIN" completion zsh
run "completion bash"       0 "$BIN" completion bash
run "completion fish"       0 "$BIN" completion fish
run "completion powershell" 0 "$BIN" completion powershell

# --- Tests: capability + file-type validation (offline) ----------------

section "capability + file-type errors (offline)"
run "codex on chat (rejected)" 2 "$BIN" -m codex hello
expect_stderr "chat reject text" 'does not support chat'
run "mini -f image (rejected)" 2 "$BIN" -m mini -f "$PNG" "describe"
expect_stderr "vision reject text" 'does not support vision'
run "unsupported file ext"  2 "$BIN" -f "$BAD" "x"
expect_stderr "unsupported text" 'unsupported file type'
run "audio with -f (rejected)" 2 "$BIN" -f /tmp/fake.mp3 "x"
expect_stderr "audio rejected msg" "does not support audio"

# --- Tests: missing API key (offline, isolated env) ---------------------

section "missing api key (offline)"
if ( unset OMI_API_KEY
  XDG_CONFIG_HOME="$WORKDIR/empty" mkdir -p "$WORKDIR/empty/omi"
  XDG_CONFIG_HOME="$WORKDIR/empty" "$BIN" "ping" \
        >"$WORKDIR/last_out" 2>"$WORKDIR/last_err"
  ec=$?
  if [ "$ec" -eq 2 ]; then exit 0; else exit 1; fi
); then
    log_pass "missing api_key exits 2"
else
    log_fail "missing api_key exits 2"
fi
expect_stderr "missing api_key msg" 'API key not set'

# --- Tests: LIVE chat ----------------------------------------------------

section "live chat (SSE / UNIFY_CHAT_WITH_AI)"
run "default chat" 0 "$BIN" "say hello in 5 words"
run "chat --no-stream" 0 "$BIN" --no-stream "say hi in 5 words"
run "chat -m mini" 0 "$BIN" -m mini "ping in 3 words"

# --- Tests: LIVE MODEL_DEFAULTS -----------------------------------------

section "live MODEL_DEFAULTS"
run "sonar auto-enables web" 0 "$BIN" -m sonar "list one tech news headline"
expect_stderr "sonar notice" 'enabling web search for sonar'
run "sonar --no-web suppresses" 0 "$BIN" -m sonar --no-web "summary of the go language in 10 words"
expect_no_stderr "no notice under --no-web" 'enabling web search'
run "grok-code on chat (rejected)" 2 "$BIN" -m grok-code "fizzbuzz in 10 words"
expect_stderr "grok-code chat rejected" 'does not support chat'

# --- Tests: LIVE raw passthrough ----------------------------------------

section "live raw model id passthrough"
run "raw id (gpt-4o-mini)" 0 "$BIN" -m gpt-4o-mini "ping in 3 words"

ec=0
"$BIN" -m bogus-alias-xyz999 "ping in 3 words" \
    >"$WORKDIR/last_out" 2>"$WORKDIR/last_err" || ec=$?
expect_stderr "raw passthrough warning" "is not a known alias"
# ec is 0 if 1min.ai accepts the unknown model, non-zero if it rejects;
# both outcomes are acceptable as long as the warning fired.

# --- Tests: LIVE attachments --------------------------------------------

section "live attachments (image / doc → /api/chat-with-ai)"
run "vision PNG"  0 "$BIN" -f "$PNG" "describe color in 3 words"
run "doc TXT"     0 "$BIN" -f "$TXT" "summarize in 5 words"
run "doc PDF"     0 "$BIN" -f "$PDF" "summarize in 5 words"

# --- Tests: LIVE upload --------------------------------------------------

section "live asset upload (multipart)"
run "upload txt fixture" 0 "$BIN" upload "$TXT"
expect_stdout "upload returned a path" '.+'

# --- Tests: LIVE transcribe (optional) -----------------------------------

section "live transcribe (/api/features SPEECH_TO_TEXT)"
if [ -n "${OMI_SMOKE_AUDIO:-}" ] && [ -r "${OMI_SMOKE_AUDIO}" ]; then
    run "transcribe audio" 0 "$BIN" transcribe "$OMI_SMOKE_AUDIO"
    expect_stdout "transcript non-empty" '.+'
else
    log_skip "transcribe" "OMI_SMOKE_AUDIO unset or unreadable"
fi

# --- Tests: LIVE code ----------------------------------------------------

section "live code (/api/features CODE_GENERATOR)"
run "code default model" 0 "$BIN" code "one-line python that prints hi"
run "code --code-model"  0 "$BIN" code --code-model gpt-5.1-codex "one-line bash echo hi"

# --- Tests: LIVE sessions ------------------------------------------------

section "live sessions (POST + DELETE /api/conversations)"
SESS="${SESS_PREFIX}-main"
run "session create on first turn" 0 "$BIN" -s "$SESS" "remember the number 42 in your reply"
run "session reuse second turn"    0 "$BIN" -s "$SESS" "what number did i ask you to remember? answer with just digits"
run "session list shows it"        0 "$BIN" session list
expect_stdout "session present in list" "$SESS"
run "session clear (DELETE)"       0 "$BIN" session clear "$SESS"
run "session clear missing (rejected)" 2 "$BIN" session clear "$SESS"
expect_stderr "missing session msg" 'no such session'

# --- Tests: LIVE clear --all ---------------------------------------------

section "live session clear --all"
SA="${SESS_PREFIX}-a"; SB="${SESS_PREFIX}-b"
run "create session A" 0 "$BIN" -s "$SA" "say letter A"
run "create session B" 0 "$BIN" -s "$SB" "say letter B"
run "clear --all -y"   0 "$BIN" session clear --all -y
"$BIN" session list >"$WORKDIR/last_out" 2>"$WORKDIR/last_err"
if grep -qE "(${SA}|${SB})" "$WORKDIR/last_out"; then
    log_fail "clear --all wiped local store"
else
    log_pass "clear --all wiped local store"
fi

# --- Tests: piped stdin --------------------------------------------------

section "piped stdin (single-turn, no REPL)"
ec=0
echo "what is 2+2? answer with just the digit." \
    | "$BIN" >"$WORKDIR/last_out" 2>"$WORKDIR/last_err" || ec=$?
if [ "$ec" -eq 0 ]; then
    log_pass "piped stdin exits 0"
    expect_stdout "piped response non-empty" '.+'
else
    log_fail "piped stdin exits 0" "exit=$ec"
fi

# --- Summary -------------------------------------------------------------

printf '\n=== summary ===\n'
printf 'PASS=%d FAIL=%d SKIP=%d\n' "$PASS" "$FAIL" "$SKIP"
[ "$FAIL" -eq 0 ]
