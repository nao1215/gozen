// Package hygiene describes repository decisions for the maintainer test runner.
// All verdicts belong to Go tests; this table contains no checking logic.
package hygiene

// Test identifies a Go test that dango must actually run, without build tags.
type Test struct {
	Package string `json:"package"`
	Name    string `json:"name"`
}

// Decision supplies the stable identity, explanation and repair guidance printed
// by dango. Tests are the executable authority, independent of ADR documents.
type Decision struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
	Fix    string `json:"fix"`
	Tests  []Test `json:"tests"`
}

// Decisions returns fresh metadata for dango and the coverage meta test.
func Decisions() []Decision {
	return []Decision{
		{"0004", "content boundary", "Published sources must contain only permitted content and portable locations.", "Remove private locations and move restricted vocabulary into the permitted production tool.", []Test{{"internal/dev/hygiene", "TestRepositoryLocations"}, {"internal/dev/docs", "TestDocumentLanguage"}}},
		{"0008", "downward dependencies and API shape", "Engine layers and loader contracts must stay reusable.", "Move dependencies downward; use Config structs and Load(fsys, path) for new filesystem loaders.", []Test{{"internal/dev/hygiene", "TestDependencyDirection"}, {"internal/dev/hygiene", "TestAPIShape"}}},
		{"0023", "default visual evidence", "Documented pictures must retain their recorded inputs and provenance.", "Review visual changes and regenerate pictures with make docs-images in a graphics environment.", []Test{{"internal/dev/docs", "TestDocImageInputs"}, {"internal/dev/docs", "TestImageManifest"}}},
		{"0041", "internal application packages", "Authors consume commands, YAML and saves rather than Go packages.", "Move application packages under internal.", []Test{{"internal/dev/hygiene", "TestApplicationPackagesAreInternal"}}},
		{"0042", "typed command execution", "Handlers may write reproduction artifacts but must not import os/exec or re-enter CLI dispatch.", "Call a shared typed operation instead of a dispatcher or parser.", []Test{{"internal/dev/hygiene", "TestCommandExecution"}, {"internal/dev/hygiene", "TestCommandExecutionGuard"}}},
		{"0047", "headless pictures", "Headless phone clips must advance without decoding picture pixels.", "Keep drawing resource preparation outside the headless Update path.", []Test{{"internal/play", "TestPhoneHeadlessClipDoesNotReadPuppetPixels"}, {"internal/play", "TestSimulatePicturesHaveTheirSize"}}},
		{"0058", "record facts without judgments", "Screen structure, layout, contrast and play measurements do not define game quality.", "Record facts without adding diagnostics or failure exits for their values.", []Test{{"cmd/gozen", "TestTraceFactsNeverFailCheck"}, {"cmd/gozen", "TestTraceMetricsFactsNeverFail"}, {"cmd/gozen", "TestCaptureDiffStatesFactsNeverFails"}}},
		{"yaml", "general-purpose YAML words", "New words need a schema, documentation, E2E coverage and two distinct examples.", "Document and exercise the YAML word; remove obsolete baseline exceptions rather than adding any.", []Test{{"internal/dev/hygiene", "TestYAMLWords"}}},
		{"speed", "speed budget", "Runtime measurements stay factual while maintainer tests bound workload costs.", "Inspect the named section and restore its allocation, heap or time budget; run graphics budgets on a desktop.", []Test{{"internal/dev/perf", "TestSpeedBudget"}, {"internal/play", "TestTracePerformance"}}},
		{"coverage", "decision coverage", "Every registered decision must have enforcing tests on the lint and CI path.", "Register existing unconditional tests and keep make lint invoking dango.", []Test{{"internal/dev/hygiene", "TestDecisionCoverage"}}},
	}
}
