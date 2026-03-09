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

// Enhance transforms prompt into a well-structured XML prompt.
// intent is optional context about the underlying goal.
// targetModel is accepted for interface compatibility but has no effect.
// maxEnhanceWords is the word count above which prompts are considered
// detailed enough to skip enhancement. Returning the prompt unchanged
// avoids adding token overhead for already-structured inputs.
const maxEnhanceWords = 300

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
	// Verb lexicons by domain — imperative verbs are strongest signals.
	codeVerbs = strset(
		"fix", "debug", "implement", "refactor", "optimize", "build",
		"deploy", "test", "lint", "migrate", "convert", "parse", "serialize",
		"compile", "format", "generate", "scaffold", "hook", "mock", "stub",
	)
	creativeVerbs = strset(
		"write", "draft", "compose", "craft", "create",
		"design", "rewrite", "edit", "proofread",
	)
	analysisVerbs = strset(
		"explain", "analyze", "analyse", "compare", "evaluate", "assess",
		"review", "summarize", "summarise", "describe", "define",
		"contrast", "discuss", "outline", "list",
	)

	// Noun lexicons help when the verb alone is ambiguous (e.g. "create a blog post" vs "create a function").
	codeNouns = strset(
		"code", "function", "func", "method", "class", "bug", "error", "script",
		"program", "api", "endpoint", "query", "sql", "test", "algorithm", "regex",
		"exception", "goroutine", "channel", "thread", "database", "schema",
		"migration", "dockerfile", "kubernetes", "interface", "struct",
		"library", "package", "module", "dependency", "benchmark", "compiler",
		"linter", "repository", "commit", "branch",
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

	// Walk tokens: collect verbs and nouns by POS tag; detect interrogatives.
	var verbs, nouns []string
	for _, tok := range doc.Tokens() {
		lower := strings.ToLower(tok.Text)
		switch {
		case strings.HasPrefix(tok.Tag, posVerb):
			verbs = append(verbs, lower)
		case strings.HasPrefix(tok.Tag, posNoun):
			nouns = append(nouns, lower)
		case strings.HasPrefix(tok.Tag, posWH):
			info.isQuestion = true
		}
	}

	// Trailing "?" also signals a question.
	if strings.HasSuffix(strings.TrimSpace(prompt), "?") {
		info.isQuestion = true
	}

	// Record the first meaningful verb — it's the strongest intent signal.
	if len(verbs) > 0 {
		info.mainVerb = verbs[0]
	}

	info.domain = classifyDomain(verbs, nouns, info.isQuestion)

	// Detect output format hints from the raw prompt.
	lower := strings.ToLower(prompt)
	for kw, desc := range outputHints {
		if strings.Contains(lower, kw) {
			info.outputHint = desc
			break
		}
	}

	info.constraints = buildConstraints(info.domain)
	return info
}

// classifyDomain scores each domain by verb + noun matches, with verbs
// weighted 2× over nouns. Questions gain extra analysis score.
func classifyDomain(verbs, nouns []string, isQuestion bool) domain {
	scores := map[domain]int{}

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
	for _, n := range nouns {
		if codeNouns[n] {
			scores[domainCode]++
		}
		if creativeNouns[n] {
			scores[domainCreative]++
		}
	}
	if isQuestion {
		scores[domainAnalysis] += 2
	}

	best, max := domainGeneral, 0
	for d, s := range scores {
		if s > max {
			max, best = s, d
		}
	}
	return best
}

func buildConstraints(d domain) []string {
	switch d {
	case domainCode:
		return []string{
			"Make only the changes necessary to fulfill the request — do not refactor unrelated code",
			"Preserve existing function signatures and public interfaces",
			"Do not add comments or documentation to code you did not change",
		}
	case domainCreative:
		return []string{
			"Match tone and voice to the intended audience",
			`Avoid generic openings and filler phrases ("In today's world...", "Certainly!")`,
		}
	case domainAnalysis:
		return []string{
			"Ground your response in concrete examples — avoid vague generalities",
			"Acknowledge relevant trade-offs or caveats where they exist",
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
		return "Expert software engineer. Write correct, idiomatic, production-quality code."
	case domainCreative:
		return "Skilled writer with expertise in crafting clear, engaging, audience-appropriate content."
	case domainAnalysis:
		if info.isQuestion {
			return "" // no role needed for simple explanatory questions
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
		b.WriteString("Approach:\n")
		b.WriteString("1. Read and understand the relevant code before making any changes\n")
		b.WriteString("2. Identify the root cause or exact requirement\n")
		b.WriteString("3. Implement the minimal, correct solution\n")
		b.WriteString("4. Verify that edge cases are handled correctly")

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
		b.WriteString("2. Support each point with a concrete example or evidence\n")
		b.WriteString("3. Acknowledge important nuances, trade-offs, or caveats")

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
		return "Explain your reasoning briefly, then show the complete, working code."
	case domainCreative:
		return "Write in flowing prose. Prioritize clarity and engagement over length."
	case domainAnalysis:
		if info.isQuestion {
			return "Answer in clear prose. Use at least one concrete example to ground the explanation."
		}
		return "Use structured prose. Include a brief summary at the end."
	default:
		return "Be concise and direct. Omit preamble."
	}
}
