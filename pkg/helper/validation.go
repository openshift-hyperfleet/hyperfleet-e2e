package helper

import (
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/core"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
)

// HasAdapterCondition checks if an adapter condition with the given type and status exists in the conditions list
func (h *Helper) HasAdapterCondition(conditions []core.AdapterCondition, condType string, status core.AdapterConditionStatus) bool {
	return hasAdapterCond(conditions, condType, status)
}

// HasResourceCondition checks if a resource condition with the given type and status exists in the conditions list
func (h *Helper) HasResourceCondition(conditions []openapi.ResourceCondition, condType string, status openapi.ResourceConditionStatus) bool {
	for _, cond := range conditions {
		if cond.Type == condType && cond.Status == status {
			return true
		}
	}
	return false
}

// GetCondition retrieves a condition by type from the conditions list
func (h *Helper) GetCondition(conditions []core.AdapterCondition, condType string) *core.AdapterCondition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// AllConditionsTrue checks if all specified condition types have status True
func (h *Helper) AllConditionsTrue(conditions []core.AdapterCondition, condTypes []string) bool {
	for _, condType := range condTypes {
		if !h.HasAdapterCondition(conditions, condType, core.AdapterConditionStatusTrue) {
			return false
		}
	}
	return true
}

// AnyConditionFalse checks if any of the specified condition types have status False
func (h *Helper) AnyConditionFalse(conditions []core.AdapterCondition, condTypes []string) bool {
	for _, condType := range condTypes {
		if h.HasAdapterCondition(conditions, condType, core.AdapterConditionStatusFalse) {
			return true
		}
	}
	return false
}

// AdapterNameToConditionType converts an adapter name to its corresponding condition type.
// Works for both cluster adapters and nodepool adapters.
// Examples:
//   - "cl-namespace" -> "ClNamespaceSuccessful"
func (h *Helper) AdapterNameToConditionType(adapterName string) string {
	// Split adapter name by "-" (e.g., "cl-namespace" -> ["cl", "namespace"])
	parts := strings.Split(adapterName, "-")

	// Capitalize each part and join them
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}

	// Add "Successful" suffix
	result.WriteString("Successful")

	return result.String()
}
