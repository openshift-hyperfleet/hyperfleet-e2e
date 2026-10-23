package util

import "maps"

// MergeLabels returns a new map with labels merged into current: keys in
// labels overwrite current. Build the Labels field of a patch request with
// it: the API replaces a resource's label set wholesale on PATCH, so the
// patch must carry the full merged set, not only the changed keys.
func MergeLabels(current, labels map[string]string) map[string]string {
	merged := make(map[string]string, len(current)+len(labels))
	maps.Copy(merged, current)
	maps.Copy(merged, labels)
	return merged
}
