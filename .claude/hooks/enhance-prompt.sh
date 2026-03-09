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

# Resolve binary: prefer PATH (works after go install), fall back to ~/go/bin.
BINARY=$(command -v better-prompter 2>/dev/null || echo "$HOME/go/bin/better-prompter")

# Bail silently if not installed yet.
if [ ! -x "$BINARY" ]; then
  exit 0
fi

# Read the full hook JSON payload from stdin.
INPUT=$(cat)

# Extract the user's prompt. Exit silently if missing or empty.
PROMPT=$(printf '%s' "$INPUT" | jq -r '.prompt // empty')
if [ -z "$PROMPT" ]; then
  exit 0
fi

# Skip very short prompts — not worth enhancing (greetings, single words, etc.)
WORD_COUNT=$(printf '%s' "$PROMPT" | wc -w)
if [ "$WORD_COUNT" -lt 4 ]; then
  exit 0
fi

# Call better-prompter in single-shot mode.
ENHANCED=$(printf '%s' "$PROMPT" | "$BINARY" --enhance 2>/dev/null) || true

# If enhancement failed or returned nothing, let the original prompt through.
if [ -z "$ENHANCED" ]; then
  exit 0
fi

# Return the XML as additionalContext — injected into Claude's context
# before it processes the user's message.
jq -n --arg ctx "$ENHANCED" '{
  hookSpecificOutput: {
    hookEventName: "UserPromptSubmit",
    additionalContext: $ctx
  }
}'
