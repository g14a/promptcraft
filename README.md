# promptcraft

**A zero-API-call MCP server that transforms raw natural language prompts into structured XML prompts following Claude's best practices.**

No LLM calls. No network requests. Pure local NLP using POS tagging and lexicon scoring.

---

## What It Does

`promptcraft` receives a raw user prompt, runs it through a rule-based NLP pipeline, and returns a structured XML prompt. The XML output is designed to be injected as context before Claude processes the original message — improving specificity, adding role framing, domain constraints, and output format guidance without modifying the user's actual request.

**Input:**
```
Implement a financial news scraper API so that I can take news based action on what specific stocks I can invest in.
```

**Output:**
```xml
<role>Expert software engineer. Design and implement correct, idiomatic, production-quality code from scratch.</role>

<instructions>
  Implement a financial news scraper API so that I can take news based action on what specific stocks I can invest in.

  Approach:
  1. Before writing any code: identify ambiguities, missing requirements, and external dependencies — state them explicitly
  2. Define core data structures and interfaces
  3. Design the solution architecture
  4. Implement each component with clear separation of concerns
  5. Handle errors explicitly and cover edge cases throughout
</instructions>

<constraints>
  - State all assumptions about unspecified requirements upfront, before writing any code
  - List any external libraries, APIs, or services the implementation requires
  - Define clear interfaces and data structures before writing implementation code
  - Keep functions small and focused — each should do one thing well
  - Handle all error paths explicitly; never silently ignore errors
  - Write production-ready code: no placeholder TODOs or stub implementations
</constraints>

<output_format>
  State any assumptions and external dependencies first. Explain key design decisions briefly, then provide the complete, working implementation. After the code, confirm it satisfies all stated requirements.
</output_format>
```

---

## Architecture

```
User prompt
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  MCP Server (JSON-RPC 2.0 over stdio)                   │
│  internal/mcp/server.go                                 │
│                                                         │
│  Methods: initialize, tools/list, tools/call, ping      │
│  Tool:    enhance_prompt                                │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  Enhancer  (internal/prompter/enhancer.go)              │
│                                                         │
│  1. prose/v2 → tokenise + POS-tag the prompt            │
│  2. analyze() → populate promptInfo struct              │
│     ├── isQuestion  (trailing ? or leading WH-word)     │
│     ├── isMultiStep (> 2 sentences)                     │
│     ├── isScrapingTask (keyword set match)              │
│     ├── domain  (scored: code / creative / analysis)    │
│     ├── isBuildTask (greenfield vs modify/fix)          │
│     ├── outputHint  (json/csv/table/etc. detected)      │
│     └── entities  (consecutive NNP/NNPS groups)        │
│  3. buildConstraints() → domain-specific rules          │
│  4. render() → assemble XML output                      │
└─────────────────────────────────────────────────────────┘
```

### Key Design Decisions

- **No API calls.** All analysis is local. The binary starts in under 50ms.
- **Idempotent.** Prompts over 300 words are returned unchanged — they are already detailed.
- **Stderr for logs.** MCP stdio stream is never polluted; all server logs go to stderr.
- **Single binary.** One compiled Go binary runs in either MCP server mode or single-shot `--enhance` mode.

---

## MCP Tool Interface

### Tool: `enhance_prompt`

**Description:** Transform a raw natural language prompt into a well-structured, XML-formatted prompt that follows Claude's best practices. Improves clarity, adds role/context/constraints, structures instructions, and specifies output format.

**Input schema:**

```json
{
  "type": "object",
  "required": ["prompt"],
  "properties": {
    "prompt": {
      "type": "string",
      "description": "The raw prompt text to enhance"
    },
    "intent": {
      "type": "string",
      "description": "Optional: the underlying goal or intent behind the prompt — helps produce a more targeted enhancement"
    },
    "target_model": {
      "type": "string",
      "description": "Optional: target Claude model tier (opus, sonnet, haiku). Accepted for interface compatibility; currently has no effect on output.",
      "enum": ["opus", "sonnet", "haiku"]
    }
  }
}
```

**Response:** Standard MCP `tools/call` result with a single `text` content block containing the XML-structured prompt.

---

## NLP Pipeline

### Step 1 — Tokenise and POS-tag

Uses `github.com/jdkato/prose/v2`. Each token gets a Penn Treebank POS tag (VB, NNP, NN, WP, etc.).

### Step 2 — Domain Classification

Scoring function with three tiers:

| Signal | Weight |
|---|---|
| POS-confirmed verb match (e.g. `fix` tagged VB) | +2 |
| POS-confirmed noun match (e.g. `function` tagged NN) | +1 |
| Fallback: token in verb lexicon regardless of POS tag | +1 |
| Leading WH-word or trailing `?` | +2 analysis |

**Why fallback scoring?** `prose/v2` frequently mis-tags capitalised sentence-opening imperative verbs as NNP (e.g. `"Implement" → NNP`). The fallback scans all tokens against verb lexicons at half weight to recover the signal.

Domains:
- `domainCode` — programming, debugging, implementation
- `domainCreative` — writing, drafting, composing
- `domainAnalysis` — explaining, comparing, evaluating
- `domainGeneral` — everything else

### Step 3 — Sub-classification

**`isBuildTask`** (code domain only): distinguishes greenfield implementation from modify/fix.

- Build verbs: `implement`, `build`, `develop`, `create`, `generate`, `scaffold`, `integrate`, `deploy`, ...
- Modify verbs: `fix`, `debug`, `refactor`, `optimize`, `lint`, ...

Greenfield prompts get architecture-first instructions and production-readiness constraints. Modify prompts get root-cause-first instructions and minimal-change constraints.

**`isScrapingTask`**: keyword set match (`scrape`, `crawl`, `fetch`, `harvest`, `extract`, `spider`). When true, injects a TOS/legal access constraint into the output.

**`isQuestion`**: trailing `?` or first token is a WH-word (WP, WRB, WDT). Mid-sentence WH-words are not counted (they are relative clauses).

**`isMultiStep`**: more than 2 sentences detected by prose sentence tokeniser.

### Step 4 — Entity Extraction

Consecutive `NNP`/`NNPS` tokens are grouped into named entity phrases (e.g. `"Economic Times"`, `"NSE"`, `"Zerodha"`). Generic abbreviations (`API`, `URL`, `HTTP`, `JSON`, `SQL`, `REST`, `SDK`) and single-character tokens are skipped. Duplicates are removed.

Entities are available in `promptInfo.entities` for template variable suggestion. (Rendering of `<variables>` blocks is in active development.)

### Step 5 — Render XML

```
<role>           — domain-specific persona (omitted for analysis questions)
<context>        — caller-supplied intent (omitted if empty)
<instructions>   — original prompt + numbered step-by-step approach
<constraints>    — domain-specific rules as bullet list
<output_format>  — what form the response should take
```

---

## Output Format by Domain

### Code — Build Task
```xml
<role>Expert software engineer. Design and implement correct, idiomatic, production-quality code from scratch.</role>

<instructions>
  {original prompt}

  Approach:
  1. Before writing any code: identify ambiguities, missing requirements, and external dependencies
  2. Define core data structures and interfaces
  3. Design the solution architecture
  4. Implement each component with clear separation of concerns
  5. Handle errors explicitly and cover edge cases throughout
</instructions>

<constraints>
  - State all assumptions about unspecified requirements upfront, before writing any code
  - List any external libraries, APIs, or services the implementation requires
  - Define clear interfaces and data structures before writing implementation code
  - Keep functions small and focused — each should do one thing well
  - Handle all error paths explicitly; never silently ignore errors
  - Write production-ready code: no placeholder TODOs or stub implementations
</constraints>

<output_format>
  State any assumptions and external dependencies first. Explain key design decisions briefly,
  then provide the complete, working implementation. After the code, confirm it satisfies all stated requirements.
</output_format>
```

### Code — Modify/Fix Task
```xml
<role>Expert software engineer. Diagnose and fix issues precisely with minimal, targeted changes.</role>

<instructions>
  {original prompt}

  Approach:
  1. Think through the problem first — state your hypothesis about the root cause before touching any code
  2. Read and understand the relevant code
  3. Confirm the root cause, then implement the minimal correct fix
  4. Verify edge cases are handled and no regressions introduced
</instructions>

<constraints>
  - Think through the root cause before changing anything — state your hypothesis first
  - Make only the changes necessary to fulfill the request — do not refactor unrelated code
  - Preserve existing function signatures and public interfaces
  - If the root cause cannot be determined from available context, state what additional information is needed
</constraints>

<output_format>
  State the root cause and your reasoning. Show the complete, corrected code.
  Confirm the fix addresses the original issue and list any edge cases verified.
</output_format>
```

### Creative
```xml
<role>Skilled writer with expertise in crafting clear, engaging, audience-appropriate content.</role>

<instructions>
  {original prompt}

  Guidelines:
  1. Open with a compelling hook — avoid generic introductions
  2. Develop each point with specific, concrete details
  3. Maintain a consistent tone throughout
  4. Close with a clear takeaway or call to action
</instructions>

<constraints>
  - Match tone and voice to the intended audience; if unspecified, state the assumed audience
  - Avoid generic openings and filler phrases ("In today's world...", "Certainly!")
</constraints>

<output_format>
  Write in flowing prose. Prioritize clarity and engagement over length.
</output_format>
```

### Analysis
```xml
<role>Expert analyst. Provide precise, well-reasoned assessments backed by concrete examples.</role>

<instructions>
  {original prompt}

  Structure your response to:
  1. State the key answer or conclusion directly upfront
  2. Support each point with a specific example, data point, or evidence
  3. For any claim you cannot support with evidence, omit it or flag it as uncertain
  4. Acknowledge important nuances, trade-offs, or caveats
</instructions>

<constraints>
  - Support each claim with a specific example, data point, or evidence — avoid vague generalities
  - If a claim cannot be supported with available evidence, omit it or flag it explicitly as uncertain
  - Acknowledge relevant trade-offs, caveats, or limitations where they exist
</constraints>

<output_format>
  Use structured prose with evidence for each point. Include a brief summary at the end.
  Note any areas where information is limited or confidence is low.
</output_format>
```

---

## Integration Modes

### 1. MCP Server (primary mode)

Runs as a stdio MCP server. Any MCP-compatible client can use the `enhance_prompt` tool.

**Claude Desktop / Claude Code (`~/.claude/claude_desktop_config.json`):**
```json
{
  "mcpServers": {
    "promptcraft": {
      "type": "stdio",
      "command": "/Users/YOUR_USER/go/bin/promptcraft"
    }
  }
}
```

Once registered, call it via the tool in any session:
```
Use the promptcraft MCP to enhance this prompt: "Refactor the auth middleware to use JWT"
```

### 2. Claude Code Hook (automatic enhancement)

The repo ships with a `UserPromptSubmit` hook that automatically pipes every prompt through `promptcraft --enhance` before Claude processes it. The structured XML is injected as `additionalContext`.

**`.claude/settings.json`** (project-level):
```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/enhance-prompt.sh",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

For **global** (all projects), copy the hook to `~/.claude/hooks/` and register it in `~/.claude/settings.json` using the same structure.

**Requirements:** `jq` on PATH.

### 3. Single-shot CLI

```bash
echo "Fix the off-by-one error in the pagination logic" | promptcraft --enhance
```

Reads prompt from stdin, writes XML to stdout. Exits immediately. Useful for shell scripts or editor integrations.

---

## Installation

**Prerequisites:** Any Recent Go installation should be fine.

```bash
# Install binary to $GOPATH/bin (typically ~/go/bin)
go install github.com/promptcraft/promptcraft/cmd/promptcraft@latest

# Verify
promptcraft --enhance <<< "Write a Go HTTP middleware for rate limiting"
```

**From source:**
```bash
git clone https://github.com/promptcraft/promptcraft
cd promptcraft
make install   # go install with version ldflags
make test      # go test -race ./...
make smoke     # quick MCP initialize round-trip test
```

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/jdkato/prose/v2` | Tokenisation, POS tagging, sentence detection |
| `github.com/deckarep/golang-set` | Internal set operations (prose transitive dep) |
| `github.com/mingrammer/commonregex` | Regex utilities (prose transitive dep) |
| `gonum.org/v1/gonum` | Linear algebra (prose transitive dep) |
| `gopkg.in/neurosnap/sentences.v1` | Sentence boundary detection (prose transitive dep) |

**Zero runtime dependencies beyond the Go standard library.** No Anthropic API key. No network access at runtime.

---

## Limitations vs. LLM-Powered Prompt Generators

This is a rule-based system. It is fast, private, and free to run, but it cannot:

- Understand semantic intent that is not signalled by keywords or POS tags
- Generate dynamic template variables based on reasoning about the domain
- Detect ethical constraints beyond the configured keyword sets
- Adapt to novel prompt structures not covered by the lexicons

For comparison: Anthropic's Workbench prompt generator uses a Claude model call to produce semantically aware enhancements. `promptcraft` achieves roughly 40-50% of that quality using only local NLP, with zero API cost and sub-50ms latency.

---

## License

MIT
