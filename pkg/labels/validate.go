package labels

import "fmt"

// ValidateLabels verifies that tests contain all required label dimensions
func ValidateLabels(testLabels []string) error {
	hasSeverity := false

	for _, label := range testLabels {
		switch label {
		// Severity dimension (required)
		case Tier0, Tier1, Tier2:
			hasSeverity = true
		// Scenario dimension (optional)
		case Negative, Performance:
			// Optional, no validation needed
		// Functionality dimension (optional)
		case Upgrade, Adapter, Auth, Capability:
			// Optional, no validation needed
		// Constraint dimension (optional)
		case Disruptive, Slow:
			// Optional, no validation needed
		}
	}

	if !hasSeverity {
		return fmt.Errorf("missing severity label (%s/%s/%s)", Tier0, Tier1, Tier2)
	}

	return nil
}
