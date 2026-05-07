#!/usr/bin/env bash
set -euo pipefail

# Sync helper for internal/models/models.json against live 1min.ai model catalogs.
# Default: print missing model IDs per feature.
# --write-snapshots: save fetched IDs under scripts/model-snapshots/*.txt
# --strict: exit non-zero when chat/code model IDs are missing from models.json
#           or speech model IDs are missing from scripts/model-snapshots/speech.txt

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
MODEL_JSON="$ROOT_DIR/internal/models/models.json"
SNAP_DIR="$ROOT_DIR/scripts/model-snapshots"

WRITE_SNAPSHOTS=0
STRICT=0

for arg in "$@"; do
  case "$arg" in
    --write-snapshots) WRITE_SNAPSHOTS=1 ;;
    --strict) STRICT=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: scripts/sync-models.sh [--write-snapshots] [--strict]

Checks live model catalogs:
  - UNIFY_CHAT_WITH_AI
  - CODE_GENERATOR
  - SPEECH_TO_TEXT

Outputs chat/code model IDs not present in internal/models/models.json apiId values,
and speech model IDs not present in scripts/model-snapshots/speech.txt.
EOF
      exit 0
      ;;
    *)
      echo "unknown arg: $arg" >&2
      exit 2
      ;;
  esac
done

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 2
fi

if [[ ! -f "$MODEL_JSON" ]]; then
  echo "missing file: $MODEL_JSON" >&2
  exit 2
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

fetch_feature() {
  local feature="$1"
  local out="$2"
  curl -fsSL "https://api.1min.ai/models?feature=${feature}" \
    | jq -r '.models[]?.modelId' \
    | awk 'NF' \
    | sort -u > "$out"
}

CHAT_IDS="$TMP_DIR/chat.txt"
CODE_IDS="$TMP_DIR/code.txt"
SPEECH_IDS="$TMP_DIR/speech.txt"
API_IDS="$TMP_DIR/api_ids.txt"

fetch_feature "UNIFY_CHAT_WITH_AI" "$CHAT_IDS"
fetch_feature "CODE_GENERATOR" "$CODE_IDS"
fetch_feature "SPEECH_TO_TEXT" "$SPEECH_IDS"

jq -r '.[].apiId' "$MODEL_JSON" | awk 'NF' | sort -u > "$API_IDS"

if [[ "$WRITE_SNAPSHOTS" -eq 1 ]]; then
  mkdir -p "$SNAP_DIR"
  cp "$CHAT_IDS" "$SNAP_DIR/chat.txt"
  cp "$CODE_IDS" "$SNAP_DIR/code.txt"
  cp "$SPEECH_IDS" "$SNAP_DIR/speech.txt"
  echo "wrote snapshots to $SNAP_DIR"
fi

print_registry_missing() {
  local label="$1"
  local feature_ids="$2"
  local missing="$TMP_DIR/missing_${label}.txt"

  comm -23 "$feature_ids" "$API_IDS" > "$missing"
  local count
  count=$(wc -l < "$missing" | tr -d ' ')
  echo "${label}: ${count} missing"
  if [[ "$count" -gt 0 ]]; then
    sed 's/^/  - /' "$missing"
  fi
  echo
  MISSING_COUNT="$count"
}

print_speech_untracked() {
  local missing="$TMP_DIR/missing_speech.txt"
  if [[ ! -f "$SNAP_DIR/speech.txt" ]]; then
    cp "$SPEECH_IDS" "$missing"
  else
    comm -23 "$SPEECH_IDS" "$SNAP_DIR/speech.txt" > "$missing"
  fi
  local count
  count=$(wc -l < "$missing" | tr -d ' ')
  echo "speech: ${count} untracked"
  if [[ "$count" -gt 0 ]]; then
    sed 's/^/  - /' "$missing"
  fi
  echo
  MISSING_COUNT="$count"
}

MISSING_COUNT=0
print_registry_missing "chat" "$CHAT_IDS"
chat_count=$MISSING_COUNT
print_registry_missing "code" "$CODE_IDS"
code_count=$MISSING_COUNT
print_speech_untracked
speech_count=$MISSING_COUNT

missing_total=$((chat_count + code_count + speech_count))
if [[ "$STRICT" -eq 1 && "$missing_total" -gt 0 ]]; then
  exit 1
fi
