package helper

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// helmValuesReadTimeout bounds the startup Helm read. The suite context carries
// only the suite-level timeout, so a stalled helm exec would otherwise block
// process 1, and every parallel process waiting on it, for the whole run.
const helmValuesReadTimeout = 60 * time.Second

// ValidateAPIEntityConfig compares the entity configuration of the deployed API
// Helm release with the E2E descriptor table. It is a read-only startup guard:
// on drift it returns one deterministic error naming the release, the namespace
// and every differing field, and it never includes the full Helm values.
//
// Every expected row must be present by kind with the same plural, parent_kind
// and on_parent_delete. Additional deployed kinds are allowed, because the API
// may serve entities this suite does not exercise. Omitted root parent fields
// compare as empty strings; an omitted child policy does not pass, because the
// expected value is not empty.
func ValidateAPIEntityConfig(
	ctx context.Context,
	namespace string,
	expected []descriptor.EntityDescriptor,
) error {
	if len(expected) == 0 {
		return errors.New("API entity drift check: the E2E descriptor table is empty")
	}
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf(
			"API entity drift check: namespace is required to read Helm release %q; set NAMESPACE",
			helmReleaseName,
		)
	}

	readCtx, cancel := context.WithTimeout(ctx, helmValuesReadTimeout)
	defer cancel()

	values, err := getAPIHelmValues(readCtx, namespace)
	if err != nil {
		return fmt.Errorf("API entity drift check: %w", err)
	}

	diffs := apiEntityDiffs(expected, values.Config.Entities)
	if len(diffs) == 0 {
		return nil
	}
	return fmt.Errorf(
		"API entity configuration drift: Helm release %q in namespace %q does not match the E2E "+
			"descriptor table in pkg/config/descriptor.go (%d difference(s)):\n  - %s\n"+
			"Redeploy the E2E infrastructure from hyperfleet-infra (e2e-kind or e2e-gcp) or update the "+
			"descriptor table",
		helmReleaseName, namespace, len(diffs), strings.Join(diffs, "\n  - "),
	)
}

// apiEntityDiffs returns one line per structural problem, then one line per
// expected field that differs. Structural problems are sorted and expected rows
// are visited in kind order, so the same drift always produces the same report.
func apiEntityDiffs(expected []descriptor.EntityDescriptor, deployed []apiEntity) []string {
	if len(deployed) == 0 {
		return []string{"config.entities is missing or empty in the deployed Helm values"}
	}

	structural, byKind := structuralEntityDiffs(deployed)

	rows := slices.Clone(expected)
	slices.SortFunc(rows, func(a, b descriptor.EntityDescriptor) int {
		return strings.Compare(a.Kind, b.Kind)
	})

	diffs := structural
	for _, want := range rows {
		got, ok := byKind[want.Kind]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: missing from config.entities", want.Kind))
			continue
		}
		fields := []struct {
			name string
			got  string
			want string
		}{
			{name: "plural", got: got.Plural, want: want.Plural},
			{name: "parent_kind", got: got.ParentKind, want: want.ParentKind},
			{name: "on_parent_delete", got: got.OnParentDelete, want: want.OnParentDelete},
		}
		for _, field := range fields {
			if field.got != field.want {
				diffs = append(diffs, fmt.Sprintf("%s: %s: deployed %q, expected %q",
					want.Kind, field.name, field.got, field.want))
			}
		}
	}
	return diffs
}

// structuralEntityDiffs indexes deployed entities by kind and reports the rows
// the field comparison cannot use: entries without a kind, duplicate kinds, and
// empty or duplicate plurals. The returned map holds the first occurrence of
// each kind.
func structuralEntityDiffs(deployed []apiEntity) ([]string, map[string]apiEntity) {
	structural := make([]string, 0)
	byKind := make(map[string]apiEntity, len(deployed))
	pluralKind := make(map[string]string, len(deployed))
	for _, entity := range deployed {
		kind := entity.Kind
		if strings.TrimSpace(kind) == "" {
			structural = append(structural, "config.entities has an entry without a kind")
			continue
		}
		if _, exists := byKind[kind]; exists {
			structural = append(structural, fmt.Sprintf("%s: duplicate kind in config.entities", kind))
			continue
		}
		byKind[kind] = entity
		plural := strings.TrimSpace(entity.Plural)
		if plural == "" {
			structural = append(structural, fmt.Sprintf("%s: plural is empty in config.entities", kind))
			continue
		}
		if other, exists := pluralKind[plural]; exists {
			structural = append(structural,
				fmt.Sprintf("%s: plural %q is already used by kind %s", kind, plural, other))
			continue
		}
		pluralKind[plural] = kind
	}
	slices.Sort(structural)
	return structural, byKind
}
