// Package prompter applies NLP-driven structural improvements to prompts.
// It has no external API dependencies and makes no network calls.
package prompter

import (
	"context"
	"fmt"
	"strings"

	"github.com/jdkato/prose/v2"
)

// Enhancer transforms raw prompts into structured XML using NLP analysis.
type Enhancer struct{}

// New creates a new Enhancer.
func New() *Enhancer { return &Enhancer{} }

// maxEnhanceWords is the word count above which prompts are considered
// detailed enough to skip enhancement. Returning the prompt unchanged
// avoids adding token overhead for already-structured inputs.
const maxEnhanceWords = 300

// Enhance transforms prompt into a well-structured XML prompt.
// intent is optional context about the underlying goal.
// targetModel is accepted for interface compatibility but has no effect.
func (e *Enhancer) Enhance(_ context.Context, prompt, intent, _ string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}
	if len(strings.Fields(prompt)) > maxEnhanceWords {
		return prompt, nil
	}

	doc, err := prose.NewDocument(prompt)
	if err != nil {
		return "", fmt.Errorf("nlp parse: %w", err)
	}

	info := analyze(prompt, intent, doc)
	return render(info), nil
}

// --- Domain classification ---------------------------------------------------

type domain int

const (
	domainGeneral  domain = iota
	domainCode            // programming, debugging, implementation
	domainCreative        // writing, drafting, composing
	domainAnalysis        // explaining, comparing, evaluating
)

// promptInfo holds NLP-derived facts about the prompt.
type promptInfo struct {
	original    string
	intent      string
	domain      domain
	isBuildTask bool // true = greenfield build; false = modify/fix existing code
	isQuestion  bool
	isMultiStep bool // more than two sentences detected
	mainVerb    string
	outputHint  string   // detected output format (json, table, etc.)
	constraints []string // domain-specific constraints
}

// POS tag prefixes used for classification.
const (
	posVerb = "VB" // VB, VBD, VBG, VBN, VBP, VBZ
	posWH   = "W"  // WP, WRB, WDT — interrogative words
	posNoun = "NN" // NN, NNS, NNP, NNPS
)

var (
	// Verb lexicons by domain — imperative verbs are the strongest signals.
	// Also used as a POS-tag-independent fallback (at half weight) because
	// prose/v2 often mis-tags capitalised sentence-opening imperatives
	// (e.g. "Implement" → NNP instead of VB).
	codeVerbs = strset(
		"fix", "debug", "implement", "refactor", "optimize", "build",
		"deploy", "test", "lint", "migrate", "convert", "parse", "serialize",
		"compile", "format", "generate", "scaffold", "hook", "mock", "stub",
		"develop", "create", "integrate", "scrape", "crawl", "ingest",
		"stream", "cache", "index", "authenticate", "authorize",
	)
	creativeVerbs = strset(
		"write", "draft", "compose", "craft",
		"design", "rewrite", "edit", "proofread",
	)
	analysisVerbs = strset(
		"explain", "analyze", "analyse", "compare", "evaluate", "assess",
		"review", "summarize", "summarise", "describe", "define",
		"contrast", "discuss", "outline", "list",
	)

	// buildVerbs are code verbs that signal a greenfield/new implementation.
	// Any code verb NOT in this set is treated as a modify/fix task.
	buildVerbs = strset(
		"implement", "build", "develop", "create", "generate", "scaffold",
		"integrate", "scrape", "crawl", "ingest", "stream", "index",
		"authenticate", "authorize", "deploy",
	)

	// Noun lexicons disambiguate when the verb alone is insufficient
	// (e.g. "create a blog post" vs "create an API").
	codeNouns = strset(
		"code", "function", "func", "method", "class", "bug", "error", "script",
		"program", "api", "endpoint", "query", "sql", "test", "algorithm", "regex",
		"exception", "goroutine", "channel", "thread", "database", "schema",
		"migration", "dockerfile", "kubernetes", "interface", "struct",
		"library", "package", "module", "dependency", "benchmark", "compiler",
		"linter", "repository", "commit", "branch",
		// commonly mis-classified technical nouns
		"scraper", "crawler", "service", "server", "client", "microservice",
		"pipeline", "sdk", "cli", "webhook", "middleware", "handler",
		"daemon", "proxy", "worker", "queue", "cache", "broker", "socket",
		"runtime", "container", "cluster", "deployment", "ingress",
	)
	creativeNouns = strset(
		"blog", "post", "article", "essay", "story", "poem", "email", "letter",
		"newsletter", "description", "caption", "pitch", "announcement",
		"proposal", "report", "tweet", "copy", "content", "headline",
	)

	// Output format keywords → instruction text.
	outputHints = map[string]string{
		"json":     "Return valid, properly indented JSON.",
		"xml":      "Return well-formed XML.",
		"yaml":     "Return valid YAML.",
		"csv":      "Return CSV with a header row and one record per line.",
		"table":    "Present results in a formatted table with clear column headers.",
		"markdown": "Format the response in Markdown.",
		"bullet":   "Use concise bullet points.",
		"list":     "Use a numbered list.",
	}
)

func strset(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// --- Analysis ----------------------------------------------------------------

func analyze(prompt, intent string, doc *prose.Document) promptInfo {
	info := promptInfo{
		original: prompt,
		intent:   intent,
	}

	// Multi-step detection: more than 2 sentences suggests a compound task.
	info.isMultiStep = len(doc.Sentences()) > 2

	tokens := doc.Tokens()

	// isQuestion: trailing "?" OR the very first token is a WH-word.
	// Mid-sentence WH-words (e.g. "on what stocks") are relative clauses —
	// they must NOT trigger isQuestion.
	trimmed := strings.TrimSpace(prompt)
	if strings.HasSuffix(trimmed, "?") {
		info.isQuestion = true
	} else if len(tokens) > 0 && strings.HasPrefix(tokens[0].Tag, posWH) {
		info.isQuestion = true
	}

	// Walk tokens: collect verbs and nouns by POS tag.
	// Also accumulate all lowercased tokens for POS-tag-independent fallback
	// scoring — prose/v2 frequently mis-tags capitalised sentence-opening
	// imperative verbs (e.g. "Implement" → NNP).
	var verbs, nouns, all []string
	for _, tok := range tokens {
		lower := strings.ToLower(tok.Text)
		all = append(all, lower)
		switch {
		case strings.HasPrefix(tok.Tag, posVerb):
			verbs = append(verbs, lower)
		case strings.HasPrefix(tok.Tag, posNoun):
			nouns = append(nouns, lower)
		}
	}

	// Record the first meaningful verb — it's the strongest intent signal.
	if len(verbs) > 0 {
		info.mainVerb = verbs[0]
	}

	info.domain = classifyDomain(verbs, nouns, all, info.isQuestion)

	// For code prompts, determine whether this is a greenfield build task or
	// a modify/fix task — each needs different framing and constraints.
	if info.domain == domainCode {
		info.isBuildTask = detectBuildTask(verbs, all)
	}

	// Detect output format hints from the raw prompt.
	lower := strings.ToLower(prompt)
	for kw, desc := range outputHints {
		if strings.Contains(lower, kw) {
			info.outputHint = desc
			break
		}
	}

	info.constraints = buildConstraints(info)
	return info
}

// classifyDomain scores each domain by verb + noun matches.
// Scoring weights:
//   - POS-confirmed verb match  → +2
//   - POS-confirmed noun match  → +1
//   - Fallback: token in verb lexicon (catches mis-tagged imperatives) → +1
//   - isQuestion                → +2 analysis
func classifyDomain(verbs, nouns, all []string, isQuestion bool) domain {
	scores := map[domain]int{}

	// POS-confirmed verbs (weight 2).
	for _, v := range verbs {
		if codeVerbs[v] {
			scores[domainCode] += 2
		}
		if creativeVerbs[v] {
			scores[domainCreative] += 2
		}
		if analysisVerbs[v] {
			scores[domainAnalysis] += 2
		}
	}
	// POS-confirmed nouns (weight 1).
	for _, n := range nouns {
		if codeNouns[n] {
			scores[domainCode]++
		}
		if creativeNouns[n] {
			scores[domainCreative]++
		}
	}
	// Fallback: score every token against verb lexicons (weight 1).
	// Recovers signal when the POS tagger mis-tags an imperative verb.
	verbSeen := make(map[string]bool, len(verbs))
	for _, v := range verbs {
		verbSeen[v] = true
	}
	for _, w := range all {
		if verbSeen[w] {
			continue // already scored above at full weight
		}
		if codeVerbs[w] {
			scores[domainCode]++
		}
		if creativeVerbs[w] {
			scores[domainCreative]++
		}
		if analysisVerbs[w] {
			scores[domainAnalysis]++
		}
	}

	if isQuestion {
		scores[domainAnalysis] += 2
	}

	best, bestScore := domainGeneral, 0
	for d, s := range scores {
		if s > bestScore {
			bestScore, best = s, d
		}
	}
	return best
}

// detectBuildTask returns true when the dominant code verb signals a
// greenfield implementation (implement, build, develop, create, …) rather
// than a modification to existing code (fix, debug, refactor, …).
// POS-confirmed verbs are checked first; the fallback scans all tokens so
// mis-tagged imperatives are still caught.
func detectBuildTask(verbs, all []string) bool {
	for _, v := range verbs {
		if buildVerbs[v] {
			return true
		}
		if codeVerbs[v] { // modify verb confirmed by POS
			return false
		}
	}
	for _, w := range all {
		if buildVerbs[w] {
			return true
		}
		if codeVerbs[w] {
			return false
		}
	}
	return true // default: treat unknown code prompts as build
}

func buildConstraints(info promptInfo) []string {
	switch info.domain {
	case domainCode:
		if info.isBuildTask {
			return []string{
				"State all assumptions about unspecified requirements upfront, before writing any code",
				"List any external libraries, APIs, or services the implementation requires",
				"Define clear interfaces and data structures before writing implementation code",
				"Keep functions small and focused — each should do one thing well",
				"Handle all error paths explicitly; never silently ignore errors",
				"Write production-ready code: no placeholder TODOs or stub implementations",
			}
		}
		return []string{
			"Think through the root cause before changing anything — state your hypothesis first",
			"Make only the changes necessary to fulfill the request — do not refactor unrelated code",
			"Preserve existing function signatures and public interfaces",
			"If the root cause cannot be determined from the available context, state what additional information is needed rather than guessing",
		}
	case domainCreative:
		return []string{
			"Match tone and voice to the intended audience; if unspecified, state the assumed audience",
			`Avoid generic openings and filler phrases ("In today's world...", "Certainly!")`,
		}
	case domainAnalysis:
		return []string{
			"Support each claim with a specific example, data point, or evidence — avoid vague generalities",
			"If a claim cannot be supported with available evidence, omit it or flag it explicitly as uncertain",
			"Acknowledge relevant trade-offs, caveats, or limitations where they exist",
		}
	default:
		return nil
	}
}

// --- Rendering ---------------------------------------------------------------

func render(info promptInfo) string {
	var b strings.Builder

	// Role — only for domains where a persona meaningfully focuses Claude.
	if role := inferRole(info); role != "" {
		fmt.Fprintf(&b, "<role>%s</role>", role)
	}

	// Context / intent — include only when caller provided it.
	if info.intent != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "<context>%s</context>", info.intent)
	}

	// Instructions — always present.
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("<instructions>\n")
	writeInstructions(&b, info)
	b.WriteString("\n</instructions>")

	// Constraints — omit for general prompts.
	if len(info.constraints) > 0 {
		b.WriteString("\n\n<constraints>")
		for _, c := range info.constraints {
			fmt.Fprintf(&b, "\n- %s", c)
		}
		b.WriteString("\n</constraints>")
	}

	// Output format — always include.
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "<output_format>%s</output_format>", inferOutputFormat(info))

	return b.String()
}

func inferRole(info promptInfo) string {
	switch info.domain {
	case domainCode:
		if info.isBuildTask {
			return "Expert software engineer. Design and implement correct, idiomatic, production-quality code from scratch."
		}
		return "Expert software engineer. Diagnose and fix issues precisely with minimal, targeted changes."
	case domainCreative:
		return "Skilled writer with expertise in crafting clear, engaging, audience-appropriate content."
	case domainAnalysis:
		if info.isQuestion {
			return "" // no persona needed for direct explanatory questions
		}
		return "Expert analyst. Provide precise, well-reasoned assessments backed by concrete examples."
	default:
		return ""
	}
}

func writeInstructions(b *strings.Builder, info promptInfo) {
	switch info.domain {
	case domainCode:
		fmt.Fprintf(b, "%s\n\n", info.original)
		if info.isBuildTask {
			b.WriteString("Approach:\n")
			b.WriteString("1. Before writing any code: identify ambiguities, missing requirements, and external dependencies — state them explicitly\n")
			b.WriteString("2. Define core data structures and interfaces\n")
			b.WriteString("3. Design the solution architecture\n")
			b.WriteString("4. Implement each component with clear separation of concerns\n")
			b.WriteString("5. Handle errors explicitly and cover edge cases throughout")
		} else {
			b.WriteString("Approach:\n")
			b.WriteString("1. Think through the problem first — state your hypothesis about the root cause before touching any code\n")
			b.WriteString("2. Read and understand the relevant code\n")
			b.WriteString("3. Confirm the root cause, then implement the minimal correct fix\n")
			b.WriteString("4. Verify edge cases are handled and no regressions introduced")
		}

	case domainCreative:
		fmt.Fprintf(b, "%s\n\n", info.original)
		b.WriteString("Guidelines:\n")
		b.WriteString("1. Open with a compelling hook — avoid generic introductions\n")
		b.WriteString("2. Develop each point with specific, concrete details\n")
		b.WriteString("3. Maintain a consistent tone throughout\n")
		b.WriteString("4. Close with a clear takeaway or call to action")

	case domainAnalysis:
		fmt.Fprintf(b, "%s\n\n", info.original)
		b.WriteString("Structure your response to:\n")
		b.WriteString("1. State the key answer or conclusion directly upfront\n")
		b.WriteString("2. Support each point with a specific example, data point, or evidence\n")
		b.WriteString("3. For any claim you cannot support with evidence, omit it or flag it as uncertain\n")
		b.WriteString("4. Acknowledge important nuances, trade-offs, or caveats")

	default:
		b.WriteString(info.original)
	}
}

func inferOutputFormat(info promptInfo) string {
	if info.outputHint != "" {
		return info.outputHint
	}
	switch info.domain {
	case domainCode:
		if info.isBuildTask {
			return "State any assumptions and external dependencies first. Explain key design decisions briefly, then provide the complete, working implementation. After the code, confirm it satisfies all stated requirements."
		}
		return "State the root cause and your reasoning. Show the complete, corrected code. Confirm the fix addresses the original issue and list any edge cases verified."
	case domainCreative:
		return "Write in flowing prose. Prioritize clarity and engagement over length."
	case domainAnalysis:
		if info.isQuestion {
			return "Answer directly and concisely. Use at least one specific example or evidence to ground each key point. If any part of the answer is uncertain, say so explicitly."
		}
		return "Use structured prose with evidence for each point. Include a brief summary at the end. Note any areas where information is limited or confidence is low."
	default:
		return "Be concise and direct. Omit preamble. If any aspect of the request is unclear, state your assumption before proceeding."
	}
}
