#!/bin/bash
# UserPromptSubmit hook — pipes every prompt through better-prompter.
#
# Reads the hook JSON from stdin, extracts the prompt, calls the binary
# in --enhance mode, and returns the XML as additionalContext so Claude
# sees the structured prompt before forming its response.
#
# Install: go install github.com/better-prompter/better-prompter/cmd/better-prompter@latest
# The binary lands at ~/go/bin/better-prompter (default GOPATH).
#
# Requirements: jq on PATH

set -euo pipefail

# Optional debug logging - set BETTER_PROMPTER_DEBUG=0 to disable
DEBUG=${BETTER_PROMPTER_DEBUG:-1}
debug_log() {
    if [ "$DEBUG" -eq 1 ]; then
        echo "[$(date '+%H:%M:%S')] better-prompter hook: $*" >&2
    fi
}

# Resolve binary: prefer PATH (works after go install), fall back to ~/go/bin.
BINARY=$(command -v better-prompter 2>/dev/null || echo "$HOME/go/bin/better-prompter")

# Bail silently if not installed yet.
if [ ! -x "$BINARY" ]; then
  debug_log "binary not found at $BINARY, skipping"
  exit 0
fi

debug_log "using binary at $BINARY"

# Read the full hook JSON payload from stdin.
INPUT=$(cat)

# Extract the user's prompt. Exit silently if missing or empty.
PROMPT=$(printf '%s' "$INPUT" | jq -r '.prompt // empty')
if [ -z "$PROMPT" ]; then
  debug_log "no prompt found in input, skipping"
  exit 0
fi

# Skip very short prompts — not worth enhancing (greetings, single words, etc.)
WORD_COUNT=$(printf '%s' "$PROMPT" | wc -w)
debug_log "processing prompt (${WORD_COUNT} words): ${PROMPT:0:50}..."
if [ "$WORD_COUNT" -lt 4 ]; then
  debug_log "prompt too short (${WORD_COUNT} words), skipping"
  exit 0
fi

# Call better-prompter in single-shot mode.
debug_log "calling better-prompter --enhance"
if [ "$DEBUG" -eq 1 ]; then
  ENHANCED=$(printf '%s' "$PROMPT" | "$BINARY" --enhance) || true
else
  ENHANCED=$(printf '%s' "$PROMPT" | "$BINARY" --enhance 2>/dev/null) || true
fi

# If enhancement failed or returned nothing, let the original prompt through.
if [ -z "$ENHANCED" ]; then
  debug_log "enhancement failed or empty, skipping"
  exit 0
fi

debug_log "enhancement successful, returning XML context"

# Return the XML as additionalContext — injected into Claude's context
# before it processes the user's message.
jq -n --arg ctx "$ENHANCED" '{
  hookSpecificOutput: {
    hookEventName: "UserPromptSubmit",
    additionalContext: $ctx
  }
}'
