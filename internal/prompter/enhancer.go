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
	original           string
	intent             string
	domain             domain
	isBuildTask        bool // true = greenfield build; false = modify/fix existing code
	isQuestion         bool
	isMultiStep        bool // more than two sentences detected
	needsStructuredOutput bool // true = benefits from Claude API structured outputs
	mainVerb           string
	outputHint         string   // detected output format (json, table, etc.)
	constraints        []string // domain-specific constraints
	entities           []string // proper-noun groups to suggest as template variables
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
		"json":     outputFormatJSON,
		"xml":      outputFormatXML,
		"yaml":     outputFormatYAML,
		"csv":      outputFormatCSV,
		"table":    outputFormatTable,
		"markdown": outputFormatMarkdown,
		"bullet":   outputFormatBullet,
		"list":     outputFormatList,
		"schema":   outputFormatSchema,
		"extract":  outputFormatExtract,
		"classify": outputFormatClassify,
		"summary":  outputFormatSummary,
	}

	// Structured output indicators - suggest Claude API structured outputs
	structuredOutputKeywords = strset(
		"schema", "extract", "parse", "structure", "format", "classify",
		"categorize", "organize", "fields", "properties", "validate",
		"conform", "standardize", "template", "pattern",
	)
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

	// Detect need for structured outputs (Claude API feature suggestion)
	for kw := range structuredOutputKeywords {
		if strings.Contains(lower, kw) {
			info.needsStructuredOutput = true
			break
		}
	}

	// Extract proper-noun groups as template variable candidates.
	// Consecutive NNP/NNPS tokens form a single entity (e.g. "Economic Times").
	info.entities = extractEntities(tokens)

	info.constraints = buildConstraints(info)
	return info
}

// extractEntities groups consecutive proper-noun tokens into named entities.
// Single-word generic proper nouns (I, API, URL, HTTP, etc.) are skipped.
func extractEntities(tokens []prose.Token) []string {
	skip := strset("i", "api", "url", "http", "https", "json", "xml", "sql", "rest", "sdk")
	var entities []string
	var current []string

	flush := func() {
		if len(current) > 0 {
			entity := strings.Join(current, " ")
			// Skip single tokens that are generic abbreviations or stop-words.
			if len(current) > 1 || (!skip[strings.ToLower(entity)] && len(entity) > 2) {
				entities = append(entities, entity)
			}
			current = current[:0]
		}
	}

	for _, tok := range tokens {
		if tok.Tag == "NNP" || tok.Tag == "NNPS" {
			current = append(current, tok.Text)
		} else {
			flush()
		}
	}
	flush()
	return dedup(entities)
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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
	baseConstraints := getBaseConstraints(info)
	entityConstraints := getEntitySpecificConstraints(info.entities)

	// Add structured output guidance if needed
	if info.needsStructuredOutput {
		structuredConstraints := []string{
			constraintStructuredFields,
			constraintStructuredData,
			constraintStructuredLabels,
		}
		return append(append(structuredConstraints, entityConstraints...), baseConstraints...)
	}

	return append(entityConstraints, baseConstraints...)
}

func getEntitySpecificConstraints(entities []string) []string {
	var constraints []string
	seenConstraints := make(map[string]bool)

	for _, entity := range entities {
		lower := strings.ToLower(entity)

		// Financial domain constraints
		if isFinancialEntity(lower) && !seenConstraints["financial"] {
			constraints = append(constraints, constraintFinancial)
			seenConstraints["financial"] = true
		}

		// Medical/health domain constraints
		if isMedicalEntity(lower) && !seenConstraints["medical"] {
			constraints = append(constraints, constraintMedical)
			seenConstraints["medical"] = true
		}

		// Social media/platform constraints
		if isSocialMediaEntity(lower) && !seenConstraints["social"] {
			constraints = append(constraints, constraintSocialMedia)
			seenConstraints["social"] = true
		}

		// Cloud platform constraints
		if isCloudPlatformEntity(lower) && !seenConstraints["cloud"] {
			constraints = append(constraints, constraintCloudPlatform)
			seenConstraints["cloud"] = true
		}
	}

	return constraints
}

func isFinancialEntity(entity string) bool {
	financialKeywords := []string{
		"stock", "ticker", "nasdaq", "nyse", "nse", "bse",
		"financial", "trading", "broker", "exchange",
		"zerodha", "robinhood", "fidelity", "schwab",
		"bloomberg", "reuters", "yahoo finance", "alpha vantage",
		"economic times", "wall street", "market",
	}
	return containsAnyKeyword(entity, financialKeywords)
}

func isMedicalEntity(entity string) bool {
	medicalKeywords := []string{
		"patient", "medical", "health", "hospital", "clinic",
		"hipaa", "phi", "diagnosis", "treatment", "prescription",
		"fda", "ehr", "emr", "healthcare", "medicine",
	}
	return containsAnyKeyword(entity, medicalKeywords)
}

func isSocialMediaEntity(entity string) bool {
	socialKeywords := []string{
		"twitter", "facebook", "instagram", "linkedin", "tiktok",
		"youtube", "reddit", "discord", "slack", "telegram",
		"social media", "post", "tweet", "message",
	}
	return containsAnyKeyword(entity, socialKeywords)
}

func isCloudPlatformEntity(entity string) bool {
	cloudKeywords := []string{
		"aws", "azure", "gcp", "google cloud", "amazon",
		"ec2", "s3", "lambda", "kubernetes", "docker",
		"cloud", "serverless", "api gateway",
	}
	return containsAnyKeyword(entity, cloudKeywords)
}

func containsAnyKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func getBaseConstraints(info promptInfo) []string {
	switch info.domain {
	case domainCode:
		if info.isBuildTask {
			return []string{
				constraintCodeAssumptions,
				constraintCodeDependencies,
				constraintCodeInterfaces,
				constraintCodeFunctions,
				constraintCodeErrors,
				constraintCodeComplete,
			}
		}
		return []string{
			constraintModifyHypothesis,
			constraintModifyFocus,
			constraintModifyInterfaces,
			constraintModifyContext,
		}
	case domainCreative:
		return []string{
			constraintCreativeAudience,
			constraintCreativeOpenings,
			constraintCreativeDetails,
		}
	case domainAnalysis:
		return []string{
			constraintAnalysisEvidence,
			constraintAnalysisUncertainty,
			constraintAnalysisLimitations,
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
	b.WriteString("<instructions>")
	writeInstructions(&b, info)
	b.WriteString("\n</instructions>")

	// Constraints — omit for general prompts.
	if len(info.constraints) > 0 {
		b.WriteString("\n\n<constraints>")
		for _, c := range info.constraints {
			fmt.Fprintf(&b, "\n  - %s", c)
		}
		b.WriteString("\n</constraints>")
	}

	// Output format — always include.
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "<output_format>\n  %s\n</output_format>", inferOutputFormat(info))

	return b.String()
}

func inferRole(info promptInfo) string {
	// Check for verb-specific specialized roles first
	if specializedRole := getSpecializedRole(info.original, info.mainVerb, info.entities, info.domain); specializedRole != "" {
		return specializedRole
	}

	// Fallback to domain-based roles
	switch info.domain {
	case domainCode:
		if info.isBuildTask {
			if info.isMultiStep {
				return roleCodeArchitect
			}
			return roleCodeBuild
		}
		return roleCodeModify
	case domainCreative:
		return roleCreative
	case domainAnalysis:
		if info.isQuestion {
			return "" // no persona needed for direct explanatory questions
		}
		return roleAnalysis
	default:
		return ""
	}
}

func getSpecializedRole(prompt, mainVerb string, entities []string, domain domain) string {
	if domain != domainCode {
		return "" // Only specialized roles for code domain initially
	}

	lower := strings.ToLower(prompt)

	// Verb-specific specialized roles
	switch mainVerb {
	case "debug", "troubleshoot":
		return roleDebugSpecialist
	case "optimize", "performance":
		return rolePerformanceExpert
	case "migrate", "convert":
		return roleMigrationSpecialist
	case "deploy", "infrastructure":
		return roleDevOpsEngineer
	case "test", "testing":
		return roleQAEngineer
	case "authenticate", "authorize":
		return roleSecurityEngineer
	}

	// Check prompt for specialized contexts
	if containsAnyKeyword(lower, []string{"scrape", "scraper", "crawl", "crawler"}) {
		return roleScrapingSpecialist
	}
	if containsAnyKeyword(lower, []string{"api", "rest", "graphql", "endpoint"}) {
		return roleAPIArchitect
	}
	if containsAnyKeyword(lower, []string{"database", "sql", "nosql", "migration", "schema"}) {
		return roleDatabaseEngineer
	}
	if containsAnyKeyword(lower, []string{"microservice", "distributed", "kubernetes", "docker"}) {
		return roleDistributedSystems
	}

	// Entity-specific role specialization
	for _, entity := range entities {
		entityLower := strings.ToLower(entity)
		if isFinancialEntity(entityLower) {
			return roleFintechEngineer
		}
		if isMedicalEntity(entityLower) {
			return roleHealthcareEngineer
		}
		if isCloudPlatformEntity(entityLower) {
			return roleCloudArchitect
		}
	}

	return ""
}

func writeInstructions(b *strings.Builder, info promptInfo) {
	switch info.domain {
	case domainCode:
		fmt.Fprintf(b, "\n  %s\n\n  Approach:\n", info.original)
		if info.isBuildTask {
			b.WriteString("  1. Before writing any code: identify ambiguities, missing requirements, and external dependencies — state them explicitly\n")
			b.WriteString("  2. Define core data structures and interfaces\n")
			b.WriteString("  3. Design the solution architecture\n")
			b.WriteString("  4. Implement each component with clear separation of concerns\n")
			b.WriteString("  5. Handle errors explicitly and cover edge cases throughout")
		} else {
			b.WriteString("  1. Think through the problem first — state your hypothesis about the root cause before touching any code\n")
			b.WriteString("  2. Read and understand the relevant code\n")
			b.WriteString("  3. Confirm the root cause, then implement the minimal correct fix\n")
			b.WriteString("  4. Verify edge cases are handled and no regressions introduced")
		}

	case domainCreative:
		fmt.Fprintf(b, "\n  %s\n\n  Guidelines:\n", info.original)
		b.WriteString("  1. Open with a compelling hook — avoid generic introductions\n")
		b.WriteString("  2. Develop each point with specific, concrete details\n")
		b.WriteString("  3. Maintain a consistent tone throughout\n")
		b.WriteString("  4. Close with a clear takeaway or call to action")

	case domainAnalysis:
		fmt.Fprintf(b, "\n  %s\n\n  Structure your response to:\n", info.original)
		b.WriteString("  1. State the key answer or conclusion directly upfront\n")
		b.WriteString("  2. Support each point with a specific example, data point, or evidence\n")
		b.WriteString("  3. For any claim you cannot support with evidence, omit it or flag it as uncertain\n")
		b.WriteString("  4. Acknowledge important nuances, trade-offs, or caveats")

	default:
		fmt.Fprintf(b, "\n  %s", info.original)
	}
}

func inferOutputFormat(info promptInfo) string {
	// Structured output suggestions for Claude API features
	if info.needsStructuredOutput {
		if info.outputHint != "" {
			return info.outputHint + " " + structuredOutputAPIHint
		}
		return structuredOutputHint
	}

	if info.outputHint != "" {
		return info.outputHint
	}

	baseFormat := getBaseOutputFormat(info)
	verbSpecific := getVerbSpecificGuidance(info.original, info.mainVerb, info.domain)

	if verbSpecific != "" {
		return verbSpecific + " " + baseFormat
	}
	return baseFormat
}

func getVerbSpecificGuidance(prompt, mainVerb string, domain domain) string {
	if domain != domainCode {
		return "" // Only add verb-specific guidance for code domain initially
	}

	// Check mainVerb first, then check prompt for verb keywords as fallback
	verbToCheck := mainVerb
	if verbToCheck == "" {
		verbToCheck = extractVerbFromPrompt(prompt)
	}

	switch verbToCheck {
	case "scrape", "crawl":
		return verbGuidanceScrape
	case "debug", "troubleshoot":
		return verbGuidanceDebug
	case "optimize", "performance":
		return verbGuidanceOptimize
	case "migrate", "convert":
		return verbGuidanceMigrate
	case "deploy", "infrastructure":
		return verbGuidanceDeploy
	case "test", "testing":
		return verbGuidanceTest
	case "authenticate", "authorize":
		return verbGuidanceAuth
	}
	return ""
}

func extractVerbFromPrompt(prompt string) string {
	lower := strings.ToLower(prompt)
	words := strings.Fields(lower)

	// Check first few words for key verbs (handles mis-tagged imperatives)
	verbMap := map[string]string{
		"scrape": "scrape", "crawl": "crawl", "debug": "debug", "troubleshoot": "troubleshoot",
		"optimize": "optimize", "migrate": "migrate", "convert": "convert",
		"deploy": "deploy", "test": "test", "authenticate": "authenticate", "authorize": "authorize",
	}

	for _, word := range words[:min(3, len(words))] { // check first 3 words
		if verb, ok := verbMap[word]; ok {
			return verb
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getBaseOutputFormat(info promptInfo) string {
	switch info.domain {
	case domainCode:
		if info.isBuildTask {
			if info.isMultiStep {
				return outputFormatCodeBuildMulti
			}
			return outputFormatCodeBuild
		}
		if info.isMultiStep {
			return outputFormatCodeModifyMulti
		}
		return outputFormatCodeModify
	case domainCreative:
		return outputFormatCreative
	case domainAnalysis:
		if info.isQuestion {
			return outputFormatAnalysisQuestion
		}
		return outputFormatAnalysisGeneral
	default:
		return outputFormatGeneral
	}
}
