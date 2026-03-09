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
		all        []string
		isQuestion bool
		want       domain
	}{
		// POS-confirmed verb signals
		{"code verb: fix", []string{"fix"}, nil, nil, false, domainCode},
		{"code verb: debug", []string{"debug"}, nil, nil, false, domainCode},
		{"code verb: implement", []string{"implement"}, nil, nil, false, domainCode},
		{"code verb: refactor", []string{"refactor"}, nil, nil, false, domainCode},
		{"creative verb: draft", []string{"draft"}, nil, nil, false, domainCreative},
		{"creative verb: compose", []string{"compose"}, nil, nil, false, domainCreative},
		{"analysis verb: explain", []string{"explain"}, nil, nil, false, domainAnalysis},
		{"analysis verb: compare", []string{"compare"}, nil, nil, false, domainAnalysis},
		{"analysis verb: evaluate", []string{"evaluate"}, nil, nil, false, domainAnalysis},
		// Noun disambiguation when verb is ambiguous or absent
		{"code nouns only", nil, []string{"function", "bug"}, nil, false, domainCode},
		{"creative nouns only", nil, []string{"blog", "article"}, nil, false, domainCreative},
		// Verb outweighs single opposing noun (verbs 2× nouns)
		{"code verb beats creative noun", []string{"fix"}, []string{"blog"}, nil, false, domainCode},
		{"creative verb beats code noun", []string{"write"}, []string{"function"}, nil, false, domainCreative},
		// Question flag adds +2 to analysis
		{"question with no lexical signal", nil, nil, nil, true, domainAnalysis},
		{"question + analysis verb amplifies", []string{"explain"}, nil, nil, true, domainAnalysis},
		// Fallback scoring: mis-tagged imperative verbs recovered via `all`
		{"fallback: implement mis-tagged", nil, nil, []string{"implement", "api"}, false, domainCode},
		{"fallback: scraper+implement beats question", nil, []string{"scraper", "api"}, []string{"implement", "scraper", "api"}, true, domainCode},
		// No signal → general
		{"no signal general", nil, nil, nil, false, domainGeneral},
		{"empty all", []string{}, []string{}, []string{}, false, domainGeneral},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyDomain(tt.verbs, tt.nouns, tt.all, tt.isQuestion)
			if got != tt.want {
				t.Errorf("classifyDomain(verbs=%v, nouns=%v, all=%v, q=%v) = %d; want %d",
					tt.verbs, tt.nouns, tt.all, tt.isQuestion, got, tt.want)
			}
		})
	}
}

// ---- detectBuildTask --------------------------------------------------------

func TestDetectBuildTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		verbs []string
		all   []string
		want  bool
	}{
		// Build verbs → true
		{"implement verb", []string{"implement"}, nil, true},
		{"build verb", []string{"build"}, nil, true},
		{"develop verb", []string{"develop"}, nil, true},
		{"create verb", []string{"create"}, nil, true},
		{"generate verb", []string{"generate"}, nil, true},
		{"scaffold verb", []string{"scaffold"}, nil, true},
		// Modify verbs → false
		{"fix verb", []string{"fix"}, nil, false},
		{"debug verb", []string{"debug"}, nil, false},
		{"refactor verb", []string{"refactor"}, nil, false},
		{"optimize verb", []string{"optimize"}, nil, false},
		// Fallback via `all` (POS mis-tagged)
		{"implement in all only", nil, []string{"implement", "api"}, true},
		{"fix in all only", nil, []string{"fix", "bug"}, false},
		// Default: unknown → build
		{"no verbs no all", nil, nil, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := detectBuildTask(tt.verbs, tt.all)
			if got != tt.want {
				t.Errorf("detectBuildTask(verbs=%v, all=%v) = %v; want %v",
					tt.verbs, tt.all, got, tt.want)
			}
		})
	}
}

// ---- buildConstraints -------------------------------------------------------

func TestBuildConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		info        promptInfo
		wantN       int
		mustContain string // one required keyword in any constraint
	}{
		{
			name:        "code build: 6 constraints including assumption + dependency guidance",
			info:        promptInfo{domain: domainCode, isBuildTask: true},
			wantN:       6,
			mustContain: "assumptions",
		},
		{
			name:        "code modify: 4 constraints including root-cause hypothesis",
			info:        promptInfo{domain: domainCode, isBuildTask: false},
			wantN:       4,
			mustContain: "root cause",
		},
		{
			name:        "creative: 2 constraints",
			info:        promptInfo{domain: domainCreative},
			wantN:       2,
			mustContain: "audience",
		},
		{
			name:        "analysis: 3 constraints including evidence requirement",
			info:        promptInfo{domain: domainAnalysis},
			wantN:       3,
			mustContain: "evidence",
		},
		{
			name:  "general: no constraints",
			info:  promptInfo{domain: domainGeneral},
			wantN: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildConstraints(tt.info)
			if len(got) != tt.wantN {
				t.Errorf("buildConstraints() returned %d items; want %d", len(got), tt.wantN)
			}
			for _, c := range got {
				if c == "" {
					t.Error("constraint must not be empty string")
				}
			}
			if tt.mustContain != "" {
				found := false
				for _, c := range got {
					if strings.Contains(strings.ToLower(c), tt.mustContain) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildConstraints() missing constraint containing %q; got %v", tt.mustContain, got)
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
		mustHave  string
	}{
		{
			name:     "code build: design/implement role",
			info:     promptInfo{domain: domainCode, isBuildTask: true},
			mustHave: "Design",
		},
		{
			name:     "code modify: diagnose/fix role",
			info:     promptInfo{domain: domainCode, isBuildTask: false},
			mustHave: "Diagnose",
		},
		{
			name:     "creative always has role",
			info:     promptInfo{domain: domainCreative},
			mustHave: "writer",
		},
		{
			name:     "analysis non-question has role",
			info:     promptInfo{domain: domainAnalysis, isQuestion: false},
			mustHave: "analyst",
		},
		{
			name:      "analysis question has no role",
			info:      promptInfo{domain: domainAnalysis, isQuestion: true},
			wantEmpty: true,
		},
		{
			name:      "general has no role",
			info:      promptInfo{domain: domainGeneral},
			wantEmpty: true,
		},
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
			if tt.mustHave != "" && !strings.Contains(got, tt.mustHave) {
				t.Errorf("inferRole() = %q; want substring %q", got, tt.mustHave)
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
		wantContain string
	}{
		{"explicit json hint", promptInfo{outputHint: "Return valid, properly indented JSON."}, "JSON"},
		{"code build: assumptions + design", promptInfo{domain: domainCode, isBuildTask: true}, "assumptions"},
		{"code build: verify requirements", promptInfo{domain: domainCode, isBuildTask: true}, "requirements"},
		{"code modify: root cause", promptInfo{domain: domainCode, isBuildTask: false}, "root cause"},
		{"code modify: confirm fix", promptInfo{domain: domainCode, isBuildTask: false}, "confirm"},
		{"creative default", promptInfo{domain: domainCreative}, "prose"},
		{"analysis question: evidence", promptInfo{domain: domainAnalysis, isQuestion: true}, "evidence"},
		{"analysis question: uncertainty", promptInfo{domain: domainAnalysis, isQuestion: true}, "uncertain"},
		{"analysis non-question: summary", promptInfo{domain: domainAnalysis, isQuestion: false}, "summary"},
		{"analysis non-question: confidence", promptInfo{domain: domainAnalysis, isQuestion: false}, "confidence"},
		{"general: assumption note", promptInfo{domain: domainGeneral}, "assumption"},
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
		mustHave []string
		mustNot  []string
	}{
		{
			name: "code modify: root-cause chain-of-thought framing",
			info: promptInfo{
				domain:      domainCode,
				isBuildTask: false,
				original:    "fix the auth bug",
				constraints: buildConstraints(promptInfo{domain: domainCode, isBuildTask: false}),
			},
			mustHave: []string{"<role>", "<instructions>", "<constraints>", "<output_format>", "root cause", "hypothesis"},
		},
		{
			name: "code build: assumption + architecture framing",
			info: promptInfo{
				domain:      domainCode,
				isBuildTask: true,
				original:    "implement a rate limiter",
				constraints: buildConstraints(promptInfo{domain: domainCode, isBuildTask: true}),
			},
			mustHave: []string{"<role>", "ambiguities", "assumptions", "<output_format>", "requirements"},
		},
		{
			name: "general domain omits role and constraints",
			info: promptInfo{
				domain:   domainGeneral,
				original: "hello world",
			},
			mustHave: []string{"<instructions>", "<output_format>"},
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
			info: promptInfo{domain: domainCode, original: "fix bug"},
			mustNot: []string{"<context>"},
		},
		{
			name:     "output format always present",
			info:     promptInfo{domain: domainCreative, original: "write a poem"},
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
			// Modify task: "fix" verb → defensive framing
			name:     "code modify: fix bug",
			prompt:   "fix the authentication bug in my Go HTTP handler",
			mustHave: []string{"<role>", "<instructions>", "<output_format>", "root cause"},
			mustNot:  []string{"Clarify requirements"},
		},
		{
			// Build task: "implement" verb → constructive framing
			name:     "code build: implement scraper",
			prompt:   "implement a financial news scraper in Go",
			mustHave: []string{"<role>", "Design", "Clarify requirements", "design decisions"},
			mustNot:  []string{"root cause"},
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
			name:     "output hint: json format detected",
			prompt:   "list all Go error handling patterns as json",
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
		{
			name:    "large prompt (>300 words) is returned unchanged",
			prompt:  strings.Repeat("word ", 301),
			mustNot: []string{"<instructions>", "<role>"},
		},
		{
			// Regression: "what" mid-sentence must not trigger isQuestion.
			// "Implement" mis-tagged by POS tagger must still classify as code build.
			name:   "regression: financial scraper API is code build, not analysis",
			prompt: "Implement a financial news scraper API so that I can take news based action on what specific stocks I can invest in.",
			mustHave: []string{
				"<role>",
				"Clarify requirements", // build-task approach step
			},
			mustNot: []string{
				"State the key answer", // analysis template
				"root cause",           // modify-task framing
			},
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

func FuzzEnhance(f *testing.F) {
	f.Add("fix the bug in the handler")
	f.Add("implement a rate limiter service")
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
		{"short_modify", "fix the bug"},
		{"medium_build", "implement a token-bucket rate limiter middleware for a Go HTTP server that limits requests per IP address"},
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
	all := []string{"implement", "fix", "debug", "function", "bug", "code", "the", "a"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyDomain(verbs, nouns, all, false)
	}
}

func BenchmarkDetectBuildTask(b *testing.B) {
	verbs := []string{"implement"}
	all := []string{"implement", "a", "rate", "limiter", "service"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detectBuildTask(verbs, all)
	}
}

func BenchmarkRender(b *testing.B) {
	cases := []struct {
		name string
		info promptInfo
	}{
		{"code_modify", promptInfo{
			original:    "fix the auth bug in the HTTP handler",
			domain:      domainCode,
			isBuildTask: false,
			constraints: buildConstraints(promptInfo{domain: domainCode, isBuildTask: false}),
		}},
		{"code_build", promptInfo{
			original:    "implement a rate limiter service",
			domain:      domainCode,
			isBuildTask: true,
			constraints: buildConstraints(promptInfo{domain: domainCode, isBuildTask: true}),
		}},
		{"creative", promptInfo{
			original:    "write a blog post about Go generics",
			domain:      domainCreative,
			constraints: buildConstraints(promptInfo{domain: domainCreative}),
		}},
		{"analysis_question", promptInfo{
			original:   "what is the difference between goroutines and threads?",
			domain:     domainAnalysis,
			isQuestion: true,
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
	infos := []promptInfo{
		{domain: domainCode, isBuildTask: true},
		{domain: domainCode, isBuildTask: false},
		{domain: domainCreative},
		{domain: domainAnalysis},
		{domain: domainGeneral},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildConstraints(infos[i%len(infos)])
	}
}
