package util

import (
	"maps"
	"testing"
)

func TestMergeLabels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current map[string]string
		labels  map[string]string
		want    map[string]string
	}{
		{
			name:    "merges and overwrites",
			current: map[string]string{"environment": "test", "team": "platform"},
			labels:  map[string]string{"team": "infra", "gate": "true"},
			want:    map[string]string{"environment": "test", "team": "infra", "gate": "true"},
		},
		{
			name:    "empty current",
			current: nil,
			labels:  map[string]string{"gate": "true"},
			want:    map[string]string{"gate": "true"},
		},
		{
			name:    "empty labels",
			current: map[string]string{"environment": "test"},
			labels:  nil,
			want:    map[string]string{"environment": "test"},
		},
		{
			name:    "both empty",
			current: nil,
			labels:  nil,
			want:    map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			merged := MergeLabels(tt.current, tt.labels)
			if !maps.Equal(merged, tt.want) {
				t.Errorf("MergeLabels(%v, %v) = %v, want %v", tt.current, tt.labels, merged, tt.want)
			}
		})
	}
}

func TestMergeLabelsReturnsNewMap(t *testing.T) {
	t.Parallel()
	current := map[string]string{"environment": "test"}
	labels := map[string]string{"gate": "true"}

	merged := MergeLabels(current, labels)

	// Merging must not alias the inputs: later changes to either map must
	// not leak into the merged set.
	current["environment"] = "mutated"
	labels["gate"] = "mutated"
	if merged["environment"] != "test" || merged["gate"] != "true" {
		t.Errorf("merged map aliases its inputs: %v", merged)
	}
}
