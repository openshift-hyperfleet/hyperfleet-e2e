package labels_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

// TestAllE2ETestsHaveRequiredLabels validates that all Ginkgo test specs
// include the three required label dimensions: Priority, Stability, and Scenario
func TestAllE2ETestsHaveRequiredLabels(t *testing.T) {
	// Find the e2e directory relative to this test file
	e2eDir := findE2EDirectory(t)

	// Collect all test specs from e2e directory
	specs, err := collectTestSpecs(e2eDir)
	if err != nil {
		t.Fatalf("Failed to collect test specs: %v", err)
	}

	if len(specs) == 0 {
		t.Fatal("No test specs found in e2e directory - this is unexpected")
	}

	t.Logf("Found %d test specs to validate", len(specs))

	// Validate each spec has required labels
	failures := []string{}
	for _, spec := range specs {
		if err := labels.ValidateLabels(spec.Labels); err != nil {
			failures = append(failures, spec.Name+": "+err.Error())
		}
	}

	// Report all failures
	if len(failures) > 0 {
		t.Errorf("Found %d test specs with missing required labels:\n  - %s",
			len(failures), strings.Join(failures, "\n  - "))
	} else {
		t.Logf("✓ All %d test specs have required labels (Severity)", len(specs))
	}
}

func TestValidateLabels(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
		labels  []string
	}{
		{name: "severity only", labels: []string{labels.Tier0}},
		{name: "severity with optional labels", labels: []string{labels.Tier2, labels.Negative, labels.Disruptive}},
		{name: "no labels", labels: nil, wantErr: "missing severity label"},
		{
			name:    "optional labels without severity",
			labels:  []string{labels.Negative},
			wantErr: "missing severity label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := labels.ValidateLabels(tt.labels)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateLabels(%v) returned error: %v", tt.labels, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateLabels(%v) error = %v, want it to contain %q", tt.labels, err, tt.wantErr)
			}
		})
	}
}

var capabilityTitle = regexp.MustCompile(`^\[Capability: [a-z0-9]+(-[a-z0-9]+)*\] \S`)

var ginkgoContainer = regexp.MustCompile(`^[FPX]?(Describe|Context|When|DescribeTable|DescribeTableSubtree)$`)

// TestCapabilitySpecTitles requires every spec in e2e/capability to be a
// package-level ginkgo.Describe with a literal [Capability: <name>] title and labels.Capability.
func TestCapabilitySpecTitles(t *testing.T) {
	dir := filepath.Join(findE2EDirectory(t), "capability")
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	var problems []string
	found := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		node, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("Failed to parse %s: %v", path, err)
		}
		fileProblems, fileFound := capabilitySpecProblems(node)
		for _, problem := range fileProblems {
			problems = append(problems, filepath.Base(path)+": "+problem)
		}
		found += fileFound
	}

	if found == 0 {
		t.Fatalf("No package-level ginkgo.Describe found in %s - the check has nothing to check", dir)
	}
	if len(problems) > 0 {
		t.Errorf("Found %d capability spec problems:\n  - %s", len(problems), strings.Join(problems, "\n  - "))
	} else {
		t.Logf("✓ All %d capability specs have a [Capability: <name>] title and labels.Capability", found)
	}
}

func TestCapabilitySpecProblems(t *testing.T) {
	const (
		title    = `"[Capability: cascade-delete] Cascades"`
		label    = "ginkgo.Label(labels.Tier0, labels.Capability)"
		badTitle = "does not start with"
		notTop   = "not a package-level"
	)
	describe := func(ttl, lbl, body string) string {
		return "ginkgo.Describe(" + ttl + ", " + lbl + ", func() {" + body + "})"
	}
	container := func(fn string) string {
		return "ginkgo." + fn + "(" + title + ", " + label + ", func() {})"
	}

	tests := []struct {
		name      string
		src       string
		wantErr   string
		wantFound int
	}{
		{name: "literal title with the capability label", src: "var _ = " + describe(title, label, ""), wantFound: 1},
		{
			name:    "suite title",
			src:     "var _ = " + describe(`"[Suite: cluster][delete] Cascades"`, label, ""),
			wantErr: badTitle, wantFound: 1,
		},
		{
			name:    "capability name with upper case",
			src:     "var _ = " + describe(`"[Capability: Cascade] Cascades"`, label, ""),
			wantErr: badTitle, wantFound: 1,
		},
		{
			name:    "no description",
			src:     "var _ = " + describe(`"[Capability: cascade-delete]"`, label, ""),
			wantErr: badTitle, wantFound: 1,
		},
		{
			name:    "computed title",
			src:     "var _ = " + describe("name", label, ""),
			wantErr: "not a string literal", wantFound: 1,
		},
		{
			name:    "missing capability label",
			src:     "var _ = " + describe(title, "ginkgo.Label(labels.Tier0)", ""),
			wantErr: "labels.Capability", wantFound: 1,
		},
		{name: "describe built in a helper", src: "func register() { " + describe(title, label, "") + " }", wantErr: notTop},
		{
			name:    "nested describe",
			src:     "var _ = " + describe(title, label, describe(`"inner"`, label, "")),
			wantErr: notTop, wantFound: 1,
		},
		{
			name:      "nested context",
			src:       "var _ = " + describe(title, label, `ginkgo.Context("inner", func() {})`),
			wantFound: 1,
		},
		{
			name:    "package-level table",
			src:     "var _ = " + container("DescribeTable"),
			wantErr: "package-level ginkgo.DescribeTable",
		},
		{name: "focused describe", src: "var _ = " + container("FDescribe"), wantErr: "package-level ginkgo.FDescribe"},
		{
			name:    "context in a helper",
			src:     "func init() { " + container("Context") + " }",
			wantErr: "outside a package-level",
		},
		{
			name:    "dot import",
			src:     `import . "github.com/onsi/ginkgo/v2"` + "\nvar _ = Describe(" + title + ", " + label + ", func() {})",
			wantErr: "imported as",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.ParseFile(token.NewFileSet(), "spec.go", "package capability\n"+tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			problems, found := capabilitySpecProblems(node)
			if found != tt.wantFound {
				t.Errorf("found %d package-level Describes, want %d", found, tt.wantFound)
			}
			joined := strings.Join(problems, "; ")
			if tt.wantErr == "" && len(problems) > 0 {
				t.Errorf("unexpected problems: %s", joined)
			}
			if tt.wantErr != "" && !strings.Contains(joined, tt.wantErr) {
				t.Errorf("problems = %q, want one containing %q", joined, tt.wantErr)
			}
		})
	}
}

// capabilitySpecProblems also returns the number of package-level Describes in the file.
func capabilitySpecProblems(node *ast.File) ([]string, int) {
	var problems []string
	for _, imp := range node.Imports {
		if imp.Path.Value == `"github.com/onsi/ginkgo/v2"` && imp.Name != nil {
			problems = append(problems, fmt.Sprintf("ginkgo is imported as %q; import it as ginkgo", imp.Name.Name))
		}
	}

	topLevel := map[*ast.CallExpr]bool{}
	packageLevel := map[*ast.CallExpr]bool{}
	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range valueSpec.Values {
				call, ok := value.(*ast.CallExpr)
				if !ok {
					continue
				}
				packageLevel[call] = true
				if isGinkgoDescribe(call) {
					topLevel[call] = true
				} else if name := ginkgoCallName(call); ginkgoContainer.MatchString(name) {
					problems = append(problems, fmt.Sprintf("package-level ginkgo.%s; capability specs use ginkgo.Describe", name))
				}
			}
		}
	}

	// Containers nested in a package-level Describe inherit its title and labels.
	inside := map[*ast.CallExpr]bool{}
	for call := range topLevel {
		ast.Inspect(call, func(n ast.Node) bool {
			if nested, ok := n.(*ast.CallExpr); ok && nested != call {
				inside[nested] = true
			}
			return true
		})
	}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isGinkgoDescribe(call) {
			if name := ginkgoCallName(call); ginkgoContainer.MatchString(name) && !inside[call] && !packageLevel[call] {
				problems = append(problems, fmt.Sprintf("ginkgo.%s is outside a package-level ginkgo.Describe", name))
			}
			return true
		}
		if !topLevel[call] {
			problems = append(problems, "ginkgo.Describe is not a package-level var initializer; nest with ginkgo.Context")
			return true
		}

		if len(call.Args) == 0 {
			problems = append(problems, "Describe has no title")
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			problems = append(problems, "Describe title is not a string literal")
			return true
		}
		title, err := strconv.Unquote(lit.Value)
		if err != nil {
			problems = append(problems, "Describe title does not unquote: "+err.Error())
			return true
		}
		if !capabilityTitle.MatchString(title) {
			problems = append(problems, fmt.Sprintf("title %q does not start with \"[Capability: <name>] \"", title))
		}
		if spec := extractSpecFromDescribe(call, ""); spec == nil || !slices.Contains(spec.Labels, labels.Capability) {
			problems = append(problems, fmt.Sprintf("%q does not carry labels.Capability", title))
		}
		return true
	})
	return problems, len(topLevel)
}

// ginkgoCallName returns X for a call spelled ginkgo.X, or "" for any other call.
func ginkgoCallName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != "ginkgo" {
		return ""
	}
	return sel.Sel.Name
}

// testSpec represents a Ginkgo test specification with its labels
type testSpec struct {
	Name   string   // Test name from ginkgo.Describe
	File   string   // Source file path
	Labels []string // Labels from ginkgo.Label()
}

// collectTestSpecs walks the e2e directory and extracts all ginkgo.Describe specs with their labels
func collectTestSpecs(e2eDir string) ([]testSpec, error) {
	var specs []testSpec

	err := filepath.Walk(e2eDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files (we only want e2e specs which use .go, not _test.go)
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the Go file
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		// Extract test specs from this file
		fileSpecs := extractSpecsFromFile(node, path)
		specs = append(specs, fileSpecs...)

		return nil
	})

	return specs, err
}

// extractSpecsFromFile extracts all ginkgo.Describe calls with labels from an AST node
func extractSpecsFromFile(node *ast.File, filePath string) []testSpec {
	var specs []testSpec

	// Walk the AST looking for ginkgo.Describe calls
	ast.Inspect(node, func(n ast.Node) bool {
		// Look for call expressions
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if this is a ginkgo.Describe call
		if !isGinkgoDescribe(callExpr) {
			return true
		}

		// Extract test name and labels
		spec := extractSpecFromDescribe(callExpr, filePath)
		if spec != nil {
			specs = append(specs, *spec)
		}

		return true
	})

	return specs
}

// isGinkgoDescribe checks if a call expression is ginkgo.Describe
func isGinkgoDescribe(call *ast.CallExpr) bool {
	return ginkgoCallName(call) == "Describe"
}

// extractSpecFromDescribe extracts test name and labels from a ginkgo.Describe call
// Expected pattern: ginkgo.Describe(name, ginkgo.Label(...), func() { ... })
func extractSpecFromDescribe(call *ast.CallExpr, filePath string) *testSpec {
	if len(call.Args) < 2 {
		return nil
	}

	// First argument should be the test name (string literal or variable)
	testName := extractStringValue(call.Args[0])
	if testName == "" {
		return nil
	}

	// Look for ginkgo.Label calls in the arguments
	var labelValues []string
	for _, arg := range call.Args[1:] {
		if labelCall, ok := arg.(*ast.CallExpr); ok {
			if isGinkgoLabel(labelCall) {
				labels := extractLabelsFromLabelCall(labelCall)
				labelValues = append(labelValues, labels...)
			}
		}
	}

	return &testSpec{
		Name:   testName,
		Labels: labelValues,
		File:   filePath,
	}
}

// isGinkgoLabel checks if a call expression is ginkgo.Label
func isGinkgoLabel(call *ast.CallExpr) bool {
	return ginkgoCallName(call) == "Label"
}

// extractLabelsFromLabelCall extracts label values from a ginkgo.Label() call
func extractLabelsFromLabelCall(call *ast.CallExpr) []string {
	var labelValues []string

	for _, arg := range call.Args {
		// Labels are typically references to constants from labels package
		// Pattern: labels.Tier0, labels.Stable, etc.
		if sel, ok := arg.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "labels" {
				// Map constant name to actual label value
				labelValue := constantToLabelValue(sel.Sel.Name)
				if labelValue != "" {
					labelValues = append(labelValues, labelValue)
				}
			}
		}
	}

	return labelValues
}

// constantToLabelValue maps label constant names to their actual values
func constantToLabelValue(constName string) string {
	// Map constant names to their string values
	// This must match the constants in pkg/labels/labels.go
	mapping := map[string]string{
		// Severity
		"Tier0": labels.Tier0,
		"Tier1": labels.Tier1,
		"Tier2": labels.Tier2,
		// Scenario
		"Negative":    labels.Negative,
		"Performance": labels.Performance,
		// Functionality
		"Upgrade":    labels.Upgrade,
		"Capability": labels.Capability,
		// Constraint
		"Disruptive": labels.Disruptive,
		"Slow":       labels.Slow,
	}

	return mapping[constName]
}

// extractStringValue extracts a string value from an expression
func extractStringValue(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		// Direct string literal
		if v.Kind == token.STRING {
			// Remove quotes from string literal
			return strings.Trim(v.Value, `"`)
		}
	case *ast.Ident:
		// Variable reference - we can't determine the value at compile time
		// Return the variable name as a placeholder
		return v.Name
	}
	return ""
}

// findE2EDirectory locates the e2e directory relative to this test file
func findE2EDirectory(t *testing.T) string {
	// This test is in pkg/labels/validate_test.go
	// e2e directory is at ../../e2e relative to this file
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Navigate up to project root and find e2e directory
	e2eDir := filepath.Join(wd, "..", "..", "e2e")
	if _, err := os.Stat(e2eDir); os.IsNotExist(err) {
		t.Fatalf("e2e directory not found at %s", e2eDir)
	}

	return e2eDir
}
