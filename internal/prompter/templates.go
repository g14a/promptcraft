// Package prompter contains template constants for prompt enhancement.
// This file contains all static text content separated from core logic.
package prompter

// --- Output Format Templates ---

const (
	// Output format instruction templates
	outputFormatJSON     = "Return valid, properly indented JSON matching the requested structure."
	outputFormatXML      = "Return well-formed XML with clear element names and proper nesting."
	outputFormatYAML     = "Return valid YAML with consistent indentation."
	outputFormatCSV      = "Return CSV with a header row and one record per line."
	outputFormatTable    = "Present results in a formatted table with clear column headers."
	outputFormatMarkdown = "Format the response in Markdown with appropriate headings and structure."
	outputFormatBullet   = "Use concise bullet points with clear, actionable items."
	outputFormatList     = "Use a numbered list with logical sequencing."
	outputFormatSchema   = "Define a clear JSON schema for the requested data structure."
	outputFormatExtract  = "Extract the requested information in structured format with labeled fields."
	outputFormatClassify = "Provide classification results with confidence levels and reasoning."
	outputFormatSummary  = "Structure the summary with distinct sections and key takeaways."

	// Structured output guidance
	structuredOutputHint    = "Structure your response with clear, labeled sections. For JSON/schema requirements, consider using Claude API structured outputs (output_config.format) to guarantee valid formatting."
	structuredOutputAPIHint = "Consider using Claude API structured outputs (output_config.format) for guaranteed schema compliance."
)

// --- Role Definitions ---

const (
	// Generic domain roles
	roleCodeBuild          = "Expert software engineer. Design and implement correct, idiomatic, production-quality code from scratch."
	roleCodeModify         = "Expert software engineer. Diagnose and fix issues precisely with minimal, targeted changes."
	roleCodeArchitect      = "Expert software architect. Design scalable systems and guide implementation across multiple components."
	roleCreative           = "Skilled writer with expertise in crafting clear, engaging, audience-appropriate content."
	roleAnalysis           = "Expert analyst. Provide precise, well-reasoned assessments backed by concrete examples."

	// Specialized roles
	roleDebugSpecialist     = "Expert debugging specialist. Systematically identify root causes and implement targeted fixes."
	rolePerformanceExpert   = "Performance optimization expert. Profile bottlenecks and implement evidence-based improvements."
	roleMigrationSpecialist = "Data migration specialist. Ensure data integrity and zero-downtime transitions."
	roleDevOpsEngineer     = "DevOps engineer. Design reliable deployment pipelines and infrastructure automation."
	roleQAEngineer         = "QA engineer. Design comprehensive test suites and validation strategies."
	roleSecurityEngineer   = "Security engineer. Implement secure authentication and fine-grained access controls."
	roleScrapingSpecialist = "Web scraping specialist. Build ethical, robust scrapers with proper rate limiting and compliance."
	roleAPIArchitect       = "API architect. Design clean, documented, and scalable API interfaces."
	roleDatabaseEngineer   = "Database engineer. Design efficient schemas and optimize query performance."
	roleDistributedSystems = "Distributed systems architect. Design resilient microservices and container orchestration."
	roleFintechEngineer    = "Financial technology engineer. Build compliant fintech solutions with proper risk management."
	roleHealthcareEngineer = "Healthcare software engineer. Develop HIPAA-compliant medical applications with data privacy."
	roleCloudArchitect     = "Cloud solutions architect. Design scalable, cost-effective cloud-native applications."
)

// --- Verb-Specific Guidance ---

const (
	verbGuidanceScrape     = "Include robots.txt compliance, rate limiting, and error handling for blocked requests."
	verbGuidanceDebug      = "Include reproduction steps, error logs, and debugging methodology."
	verbGuidanceOptimize   = "Include benchmarks, performance metrics, and before/after comparisons."
	verbGuidanceMigrate    = "Include data integrity checks, rollback procedures, and migration validation."
	verbGuidanceDeploy     = "Include environment setup, deployment steps, and monitoring considerations."
	verbGuidanceTest       = "Include test cases, coverage reports, and edge case validation."
	verbGuidanceAuth       = "Include security considerations, token handling, and access control patterns."
)

// --- Base Output Format Templates ---

const (
	outputFormatCodeBuild        = "State any assumptions and external dependencies first. Explain key design decisions briefly, then provide the complete, working implementation. After the code, confirm it satisfies all stated requirements."
	outputFormatCodeBuildMulti   = "Break down the implementation into numbered steps. " + outputFormatCodeBuild
	outputFormatCodeModify       = "State the root cause and your reasoning. Show the complete, corrected code. Confirm the fix addresses the original issue and list any edge cases verified."
	outputFormatCodeModifyMulti  = "Address each issue systematically in numbered steps. " + outputFormatCodeModify
	outputFormatCreative         = "Write in flowing prose. Prioritize clarity and engagement over length."
	outputFormatAnalysisQuestion = "Answer directly and concisely. Support each key point with specific examples. Flag any uncertain claims explicitly."
	outputFormatAnalysisGeneral  = "Use structured prose with evidence for each point. Include a brief summary at the end. Note any areas where information is limited or confidence is low."
	outputFormatGeneral          = "Be concise and direct. State any assumptions upfront before proceeding with your response."
)

// --- Constraint Templates ---

const (
	// Structured output constraints
	constraintStructuredFields     = "Use consistent field names and data types throughout your response"
	constraintStructuredData       = "Provide complete data for all requested fields when information is available"
	constraintStructuredLabels     = "Use clear, descriptive labels for any categories or classifications"

	// Code build constraints
	constraintCodeAssumptions      = "State all assumptions about unspecified requirements upfront, before writing code"
	constraintCodeDependencies     = "List the external libraries, APIs, or services the implementation will use"
	constraintCodeInterfaces       = "Define clear interfaces and data structures before implementation details"
	constraintCodeFunctions        = "Write focused functions that accomplish one specific task well"
	constraintCodeErrors           = "Handle error conditions explicitly with appropriate error messages"
	constraintCodeComplete         = "Provide complete, working code without placeholder TODOs or incomplete sections"

	// Code modify constraints
	constraintModifyHypothesis     = "State your hypothesis about the root cause before making changes"
	constraintModifyFocus          = "Focus changes on addressing the specific issue described in the request"
	constraintModifyInterfaces     = "Preserve existing function signatures and public interfaces unless modification is required"
	constraintModifyContext        = "Request additional context when the root cause cannot be determined from available information"

	// Creative constraints
	constraintCreativeAudience     = "Match tone and voice to the intended audience; state your assumed audience if unspecified"
	constraintCreativeOpenings     = "Use engaging openings that draw the reader in immediately"
	constraintCreativeDetails      = "Develop each point with specific, concrete details and examples"

	// Analysis constraints
	constraintAnalysisEvidence     = "Support each claim with specific examples, data points, or concrete evidence"
	constraintAnalysisUncertainty  = "Flag any claims as uncertain when supporting evidence is limited or unavailable"
	constraintAnalysisLimitations  = "Acknowledge relevant trade-offs, caveats, or limitations where they apply to your analysis"

	// Entity-specific constraints
	constraintFinancial            = "Consider data licensing, rate limits, and financial data compliance requirements"
	constraintMedical              = "Ensure HIPAA compliance, data privacy, and medical data handling requirements"
	constraintSocialMedia          = "Respect platform rate limits, API terms of service, and user privacy"
	constraintCloudPlatform        = "Include error handling for cloud service limits, authentication, and region considerations"
)