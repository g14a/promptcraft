package prompter

import (
	"context"
	"strings"
	"testing"
)

// ---- classifyDomain ---------------------------------------------------------

func TestClassifyDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		verbs      []string
		nouns      []string
		isQuestion bool
		want       domain
	}{
		// Strong verb signals
		{"code verb: fix", []string{"fix"}, nil, false, domainCode},
		{"code verb: debug", []string{"debug"}, nil, false, domainCode},
		{"code verb: implement", []string{"implement"}, nil, false, domainCode},
		{"code verb: refactor", []string{"refactor"}, nil, false, domainCode},
		{"creative verb: write", []string{"write"}, nil, false, domainCreative},
		{"creative verb: draft", []string{"draft"}, nil, false, domainCreative},
		{"creative verb: compose", []string{"compose"}, nil, false, domainCreative},
		{"analysis verb: explain", []string{"explain"}, nil, false, domainAnalysis},
		{"analysis verb: compare", []string{"compare"}, nil, false, domainAnalysis},
		{"analysis verb: evaluate", []string{"evaluate"}, nil, false, domainAnalysis},
		// Noun disambiguation when verb is ambiguous or absent
		{"code nouns only", nil, []string{"function", "bug"}, false, domainCode},
		{"creative nouns only", nil, []string{"blog", "article"}, false, domainCreative},
		// Verb outweighs single opposing noun (verbs 2× nouns)
		{"code verb beats creative noun", []string{"fix"}, []string{"blog"}, false, domainCode},
		{"creative verb beats code noun", []string{"write"}, []string{"function"}, false, domainCreative},
		// Question flag adds +2 to analysis
		{"question with no lexical signal", nil, nil, true, domainAnalysis},
		{"question + analysis verb amplifies", []string{"explain"}, nil, true, domainAnalysis},
		// No signal → general
		{"no signal general", nil, nil, false, domainGeneral},
		{"empty all", []string{}, []string{}, false, domainGeneral},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyDomain(tt.verbs, tt.nouns, tt.isQuestion)
			if got != tt.want {
				t.Errorf("classifyDomain(%v, %v, %v) = %d; want %d",
					tt.verbs, tt.nouns, tt.isQuestion, got, tt.want)
			}
		})
	}
}

// ---- buildConstraints -------------------------------------------------------

func TestBuildConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d     domain
		wantN int
	}{
		{domainCode, 3},
		{domainCreative, 2},
		{domainAnalysis, 2},
		{domainGeneral, 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("domain", func(t *testing.T) {
			t.Parallel()
			got := buildConstraints(tt.d)
			if len(got) != tt.wantN {
				t.Errorf("buildConstraints(%d) returned %d items; want %d", tt.d, len(got), tt.wantN)
			}
			for _, c := range got {
				if c == "" {
					t.Error("constraint must not be empty string")
				}
			}
		})
	}
}

// ---- inferRole --------------------------------------------------------------

func TestInferRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		info      promptInfo
		wantEmpty bool
	}{
		{"code always has role", promptInfo{domain: domainCode}, false},
		{"creative always has role", promptInfo{domain: domainCreative}, false},
		{"analysis non-question has role", promptInfo{domain: domainAnalysis, isQuestion: false}, false},
		// Questions are self-explanatory; a role adds noise, not value.
		{"analysis question has no role", promptInfo{domain: domainAnalysis, isQuestion: true}, true},
		{"general has no role", promptInfo{domain: domainGeneral}, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := inferRole(tt.info)
			if tt.wantEmpty && got != "" {
				t.Errorf("inferRole() = %q; want empty string", got)
			}
			if !tt.wantEmpty && got == "" {
				t.Error("inferRole() = empty; want non-empty role")
			}
		})
	}
}

// ---- inferOutputFormat ------------------------------------------------------

func TestInferOutputFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		info        promptInfo
		wantContain string // substring the result must contain
	}{
		{"explicit json hint", promptInfo{outputHint: "Return valid, properly indented JSON."}, "JSON"},
		{"code default", promptInfo{domain: domainCode}, "code"},
		{"creative default", promptInfo{domain: domainCreative}, "prose"},
		{"analysis question default", promptInfo{domain: domainAnalysis, isQuestion: true}, "example"},
		{"analysis non-question default", promptInfo{domain: domainAnalysis, isQuestion: false}, "summary"},
		{"general default", promptInfo{domain: domainGeneral}, "concise"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := inferOutputFormat(tt.info)
			if got == "" {
				t.Fatal("inferOutputFormat() returned empty string")
			}
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tt.wantContain)) {
				t.Errorf("inferOutputFormat() = %q; want substring %q", got, tt.wantContain)
			}
		})
	}
}

// ---- render -----------------------------------------------------------------

func TestRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		info     promptInfo
		mustHave []string // XML tags/substrings that MUST appear
		mustNot  []string // substrings that must NOT appear
	}{
		{
			name: "code domain has all major sections",
			info: promptInfo{
				domain:      domainCode,
				original:    "fix the auth bug",
				constraints: buildConstraints(domainCode),
			},
			mustHave: []string{"<role>", "</role>", "<instructions>", "</instructions>", "<constraints>", "<output_format>"},
		},
		{
			name: "general domain omits role and constraints",
			info: promptInfo{
				domain:   domainGeneral,
				original: "hello world",
			},
			mustHave: []string{"<instructions>", "</instructions>", "<output_format>"},
			mustNot:  []string{"<role>", "<constraints>"},
		},
		{
			name: "intent produces context tag",
			info: promptInfo{
				domain:   domainGeneral,
				original: "hello",
				intent:   "validate the system",
			},
			mustHave: []string{"<context>validate the system</context>"},
		},
		{
			name: "no intent means no context tag",
			info: promptInfo{
				domain:   domainCode,
				original: "fix bug",
				intent:   "",
			},
			mustNot: []string{"<context>"},
		},
		{
			name: "output format always present",
			info: promptInfo{domain: domainCreative, original: "write a poem"},
			mustHave: []string{"<output_format>", "</output_format>"},
		},
		{
			name: "explicit output hint used verbatim",
			info: promptInfo{
				domain:     domainGeneral,
				original:   "list things",
				outputHint: "Return valid, properly indented JSON.",
			},
			mustHave: []string{"Return valid, properly indented JSON."},
		},
		{
			name: "original prompt appears inside instructions",
			info: promptInfo{
				domain:   domainCode,
				original: "implement a rate limiter",
			},
			mustHave: []string{"implement a rate limiter"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := render(tt.info)

			for _, s := range tt.mustHave {
				if !strings.Contains(got, s) {
					t.Errorf("render() missing %q\nfull output:\n%s", s, got)
				}
			}
			for _, s := range tt.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("render() unexpectedly contains %q\nfull output:\n%s", s, got)
				}
			}
		})
	}
}

// ---- Enhance — integration (uses prose NLP) ---------------------------------

func TestEnhance(t *testing.T) {
	// Not parallel: prose init is safe but no need to race here.
	e := New()
	ctx := context.Background()

	tests := []struct {
		name     string
		prompt   string
		intent   string
		wantErr  bool
		mustHave []string
		mustNot  []string
	}{
		{
			name:    "empty prompt returns error",
			prompt:  "",
			wantErr: true,
		},
		{
			name:    "whitespace-only returns error",
			prompt:  "   \t\n",
			wantErr: true,
		},
		{
			name:     "code prompt: fix bug",
			prompt:   "fix the authentication bug in my Go HTTP handler",
			mustHave: []string{"<instructions>", "</instructions>", "<output_format>"},
		},
		{
			name:     "creative prompt: write blog",
			prompt:   "write a blog post about Go concurrency patterns",
			mustHave: []string{"<instructions>", "<output_format>"},
		},
		{
			name:     "question prompt: goroutines",
			prompt:   "what is the difference between goroutines and OS threads?",
			mustHave: []string{"<instructions>", "<output_format>"},
		},
		{
			name:     "analysis prompt: compare",
			prompt:   "compare microservices and monolithic architectures",
			mustHave: []string{"<instructions>", "<output_format>"},
		},
		{
			name:   "intent injects context tag",
			prompt: "explain recursion",
			intent: "for a junior developer with no CS background",
			mustHave: []string{
				"<context>for a junior developer with no CS background</context>",
				"<instructions>",
			},
		},
		{
			name:    "output hint: json format detected",
			prompt:  "list all Go error handling patterns as json",
			mustHave: []string{"<output_format>"},
		},
		{
			name:     "targetModel arg is ignored (no error)",
			prompt:   "debug this function",
			mustHave: []string{"<instructions>"},
		},
		{
			name:     "result always has instructions and output_format",
			prompt:   "do something useful",
			mustHave: []string{"<instructions>", "</instructions>", "<output_format>", "</output_format>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.Enhance(ctx, tt.prompt, tt.intent, "opus")

			if tt.wantErr {
				if err == nil {
					t.Errorf("Enhance(%q) expected error; got nil", tt.prompt)
				}
				return
			}
			if err != nil {
				t.Fatalf("Enhance(%q) unexpected error: %v", tt.prompt, err)
			}
			if got == "" {
				t.Fatal("Enhance() returned empty string")
			}
			for _, s := range tt.mustHave {
				if !strings.Contains(got, s) {
					t.Errorf("Enhance() missing %q\nfull output:\n%s", s, got)
				}
			}
			for _, s := range tt.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("Enhance() unexpected %q\nfull output:\n%s", s, got)
				}
			}
		})
	}
}

// ---- Fuzz -------------------------------------------------------------------

// FuzzEnhance checks that Enhance never panics on arbitrary input and that
// any non-empty prompt always produces a result with <instructions>.
func FuzzEnhance(f *testing.F) {
	// Seed corpus covering all domain paths.
	f.Add("fix the bug in the handler")
	f.Add("write a blog post about Go")
	f.Add("what is a goroutine?")
	f.Add("explain the difference between value and pointer receivers")
	f.Add("")
	f.Add("   ")
	f.Add("!!!???")
	f.Add("a")

	e := New()
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, prompt string) {
		result, err := e.Enhance(ctx, prompt, "", "")

		if strings.TrimSpace(prompt) == "" {
			if err == nil {
				t.Errorf("expected error for blank prompt %q; got result %q", prompt, result)
			}
			return
		}

		if err != nil {
			t.Errorf("unexpected error for prompt %q: %v", prompt, err)
			return
		}
		if !strings.Contains(result, "<instructions>") {
			t.Errorf("result missing <instructions> for prompt %q\noutput:\n%s", prompt, result)
		}
		if !strings.Contains(result, "</instructions>") {
			t.Errorf("result missing </instructions> for prompt %q\noutput:\n%s", prompt, result)
		}
		if !strings.Contains(result, "<output_format>") {
			t.Errorf("result missing <output_format> for prompt %q\noutput:\n%s", prompt, result)
		}
	})
}

// ---- Benchmarks -------------------------------------------------------------

func BenchmarkEnhance(b *testing.B) {
	e := New()
	ctx := context.Background()

	cases := []struct {
		name   string
		prompt string
	}{
		{"short_code", "fix the bug"},
		{"medium_code", "implement a token-bucket rate limiter middleware for a Go HTTP server that limits requests per IP address"},
		{"short_question", "what is a goroutine?"},
		{"medium_creative", "write a blog post about the future of AI in software engineering"},
		{"long_analysis", "analyze the trade-offs between microservices and monolithic architectures, covering deployment complexity, team autonomy, data consistency, and operational overhead"},
	}

	for _, c := range cases {
		c := c
		b.Run(c.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = e.Enhance(ctx, c.prompt, "", "")
			}
		})
	}
}

func BenchmarkClassifyDomain(b *testing.B) {
	verbs := []string{"fix", "debug", "implement"}
	nouns := []string{"function", "bug", "code"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyDomain(verbs, nouns, false)
	}
}

func BenchmarkRender(b *testing.B) {
	cases := []struct {
		name string
		info promptInfo
	}{
		{"code", promptInfo{
			original:    "fix the auth bug in the HTTP handler",
			domain:      domainCode,
			constraints: buildConstraints(domainCode),
		}},
		{"creative", promptInfo{
			original:    "write a blog post about Go generics",
			domain:      domainCreative,
			constraints: buildConstraints(domainCreative),
		}},
		{"analysis_question", promptInfo{
			original:   "what is the difference between goroutines and threads?",
			domain:     domainAnalysis,
			isQuestion: true,
		}},
		{"with_intent", promptInfo{
			original: "explain closures",
			domain:   domainAnalysis,
			intent:   "for a developer new to Go",
		}},
	}

	for _, c := range cases {
		c := c
		b.Run(c.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				render(c.info)
			}
		})
	}
}

func BenchmarkBuildConstraints(b *testing.B) {
	domains := []domain{domainCode, domainCreative, domainAnalysis, domainGeneral}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildConstraints(domains[i%len(domains)])
	}
}
