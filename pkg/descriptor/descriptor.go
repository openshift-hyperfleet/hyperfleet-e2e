// Package descriptor describes the resources exercised by the E2E suite.
// It has no dependency on suite configuration or a deployed environment.
package descriptor

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// EntityDescriptor combines routing metadata with the suite's runtime settings.
// Runtime settings are supplied by the table in pkg/config.
type EntityDescriptor struct {
	Kind             string
	Plural           string
	ParentKind       string
	ParentPlural     string // resolved by NewCatalog
	OnParentDelete   string
	RequiredAdapters []string
	Cleanup          CleanupPolicy
	Thresholds       EntityThresholds
	Timeouts         EntityTimeouts
}

type EntityTimeouts struct {
	Reconciled time.Duration
	Deleted    time.Duration
}

type EntityThresholds struct {
	Read             time.Duration
	List             time.Duration
	Create           time.Duration
	Update           time.Duration
	Delete           time.Duration
	CreateReconciled time.Duration
	UpdateReconciled time.Duration
	Deleted          time.Duration
	CascadeDeleted   time.Duration
}

// CleanupPolicy preserves the existing suites' teardown contracts. These are
// test cleanup choices, not the API's deletion semantics. In particular,
// IgnoreConflict is a transitional exception, not a default for reference targets.
type CleanupPolicy struct {
	ChildKinds        []string // explicitly delete these children before the resource
	WaitForHardDelete bool
	IgnoreNotFound    bool
	IgnoreConflict    bool
}

// Catalog owns its entries; lookups return copies so a test cannot change
// another test's adapters or cleanup policy.
type Catalog struct {
	entries map[string]EntityDescriptor
}

func NewCatalog(entries []EntityDescriptor) (*Catalog, error) {
	c := &Catalog{entries: make(map[string]EntityDescriptor, len(entries))}
	plurals := make(map[string]bool, len(entries))
	for _, entity := range entries {
		if strings.TrimSpace(entity.Kind) == "" || !validSegment(entity.Plural) {
			return nil, fmt.Errorf("entity kind and a valid plural are required: %q/%q", entity.Kind, entity.Plural)
		}
		if _, exists := c.entries[entity.Kind]; exists {
			return nil, fmt.Errorf("duplicate entity kind %q", entity.Kind)
		}
		if plurals[entity.Plural] {
			return nil, fmt.Errorf("duplicate entity plural %q", entity.Plural)
		}
		plurals[entity.Plural] = true
		c.entries[entity.Kind] = entity.clone()
	}
	for kind, entity := range c.entries {
		if entity.ParentKind != "" {
			parent, ok := c.entries[entity.ParentKind]
			if !ok {
				return nil, fmt.Errorf("entity %q has unknown parent %q", kind, entity.ParentKind)
			}
			if entity.ParentPlural != "" && entity.ParentPlural != parent.Plural {
				return nil, fmt.Errorf("entity %q parent plural does not match %q", kind, parent.Plural)
			}
			entity.ParentPlural = parent.Plural
		}
		if err := entity.Validate(); err != nil {
			return nil, err
		}
		seen := map[string]bool{kind: true}
		for ancestor := entity.ParentKind; ancestor != ""; ancestor = c.entries[ancestor].ParentKind {
			if seen[ancestor] {
				return nil, fmt.Errorf("entity %q has cyclic parent relationships", kind)
			}
			seen[ancestor] = true
		}
		for _, childKind := range entity.Cleanup.ChildKinds {
			child, ok := c.entries[childKind]
			if !ok || child.ParentKind != kind {
				return nil, fmt.Errorf("cleanup child %q does not belong to %q", childKind, kind)
			}
		}
		c.entries[kind] = entity
	}
	return c, nil
}

func (c *Catalog) Lookup(kind string) (EntityDescriptor, error) {
	if c != nil {
		if entity, ok := c.entries[kind]; ok {
			return entity.clone(), nil
		}
	}
	return EntityDescriptor{}, fmt.Errorf("entity kind %q not found in descriptor catalog", kind)
}

// MustGet is for known kind literals in specs. Use Lookup for runtime input.
func (c *Catalog) MustGet(kind string) EntityDescriptor {
	entity, err := c.Lookup(kind)
	if err != nil {
		panic(err)
	}
	return entity
}

func (e EntityDescriptor) clone() EntityDescriptor {
	e.RequiredAdapters = slices.Clone(e.RequiredAdapters)
	e.Cleanup.ChildKinds = slices.Clone(e.Cleanup.ChildKinds)
	return e
}

// Validate checks routing metadata and the runtime settings the entity is waited
// on with: timeouts are not negative, required adapters come with a reconciled
// timeout, and waiting for hard-delete comes with a deleted timeout.
func (e EntityDescriptor) Validate() error {
	if strings.TrimSpace(e.Kind) == "" || !validSegment(e.Plural) {
		return fmt.Errorf("entity kind and a valid plural are required: %q/%q", e.Kind, e.Plural)
	}
	if e.ParentKind == "" {
		if e.ParentPlural != "" || e.OnParentDelete != "" {
			return fmt.Errorf("root entity %q has parent metadata", e.Kind)
		}
	} else if !validSegment(e.ParentPlural) || (e.OnParentDelete != "cascade" && e.OnParentDelete != "restrict") {
		return fmt.Errorf("entity %q requires a parent plural and cascade/restrict deletion policy", e.Kind)
	}
	if e.Timeouts.Reconciled < 0 || e.Timeouts.Deleted < 0 {
		return fmt.Errorf("entity %q timeouts must not be negative, got %+v", e.Kind, e.Timeouts)
	}
	if len(e.RequiredAdapters) > 0 && e.Timeouts.Reconciled == 0 {
		return fmt.Errorf("entity %q has required adapters but no reconciled timeout", e.Kind)
	}
	if e.Cleanup.WaitForHardDelete && e.Timeouts.Deleted <= 0 {
		return fmt.Errorf(
			"entity %q requires a positive deleted timeout when waiting for hard-delete, got %s",
			e.Kind,
			e.Timeouts.Deleted,
		)
	}
	return nil
}

// CollectionPath returns a path relative to the client's API prefix. It preserves
// the existing clients' path concatenation; IDs are passed through to the API.
func (e EntityDescriptor) CollectionPath(parentID string) string {
	if e.ParentKind == "" {
		return e.Plural
	}
	return e.ParentPlural + "/" + parentID + "/" + e.Plural
}

func (e EntityDescriptor) ResourcePath(parentID, resourceID string) string {
	return e.CollectionPath(parentID) + "/" + resourceID
}

func validSegment(value string) bool {
	return value != "" && value != "." && value != ".." && url.PathEscape(value) == value
}
