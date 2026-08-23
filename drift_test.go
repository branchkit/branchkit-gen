package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func testSpec() *Spec {
	return &Spec{
		APIVersion: "0.5.0",
		Methods: map[string]*SpecMethod{
			"collection.get":     {Name: "collection.get", Since: "0.1.0"},
			"collection.put":     {Name: "collection.put", Since: "0.3.0"},
			"old.thing":          {Name: "old.thing", Since: "0.1.0", DeprecatedIn: "0.4.0"},
			"removed.thing":      {Name: "removed.thing", Since: "0.1.0", RemovedIn: "0.5.0"},
			"deprecated.removed": {Name: "deprecated.removed", Since: "0.1.0", DeprecatedIn: "0.3.0", RemovedIn: "0.5.0"},
		},
		Privileges: map[string]bool{
			"dispatch":      true,
			"accessibility": true,
		},
	}
}

func driftIssues(t *testing.T, m *PluginManifest, called []CalledMethod) []Issue {
	t.Helper()
	return CheckDrift(m, called, testSpec())
}

// The bug this guards: the manifest struct read a `capabilities` JSON key
// the platform had renamed to `privileges`, so Privileges was always empty
// once a real file was parsed. Every test set the Go field directly, which
// never exercises the tag — so the drift check silently validated nothing
// for however long, and dispatch_via reported a false error on every plugin
// that used it. Parse from JSON, the way production does.
func TestManifest_privilegesParseFromJSON(t *testing.T) {
	raw := []byte(`{
		"id": "demo-plugin",
		"min_api_version": "0.1.0",
		"privileges": ["dispatch", "apps"]
	}`)
	var m PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m.Privileges) != 2 || m.Privileges[0] != "dispatch" {
		t.Fatalf("privileges did not parse from JSON: %+v", m.Privileges)
	}

	// dispatch_via is satisfied by a privilege that came off the wire.
	m.DispatchVia = "direct"
	m.DispatchPrefixes = []string{"demo."}
	for _, i := range Validate(&m, nil) {
		if i.Field == "dispatch_via" && i.Severity == SeverityError {
			t.Errorf("dispatch_via rejected despite the dispatch privilege: %s", i.Message)
		}
	}
}

// `capabilities` is the dead name. A manifest still using it must not pass
// the typo guard in silence, or the rename above is invisible to authors.
func TestValidate_deadCapabilitiesKeyIsFlagged(t *testing.T) {
	raw := map[string]any{"capabilities": []any{"dispatch"}}
	m := minimalValid()
	found := false
	for _, i := range Validate(m, raw) {
		if i.Field == "capabilities" {
			found = true
		}
	}
	if !found {
		t.Error("a manifest using the old `capabilities` key should be flagged as unrecognized")
	}
}

func TestDrift_unknownPrivilege(t *testing.T) {
	m := minimalValid()
	m.Privileges = []string{"dispatch", "made-up-privilege"}
	issues := driftIssues(t, m, nil)
	found := false
	for _, i := range issues {
		if strings.HasPrefix(i.Field, "privileges[") && i.Severity == SeverityError &&
			strings.Contains(i.Message, "made-up-privilege") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for unknown privilege, got %+v", issues)
	}
}

func TestDrift_minAPIVersionAheadOfSpec(t *testing.T) {
	m := minimalValid()
	m.MinAPIVersion = "1.0.0"
	issues := driftIssues(t, m, nil)
	found := false
	for _, i := range issues {
		if i.Field == "min_api_version" && i.Severity == SeverityInfo {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info note when min_api_version > spec version, got %+v", issues)
	}
}

func TestDrift_methodSinceNewerThanMin(t *testing.T) {
	m := minimalValid()
	m.MinAPIVersion = "0.2.0"
	called := []CalledMethod{
		{Name: "collection.put", File: "src/main.go", Line: 42}, // since 0.3.0 > min 0.2.0
	}
	issues := driftIssues(t, m, called)
	found := false
	for _, i := range issues {
		if i.Severity == SeverityError && strings.Contains(i.Message, "introduced in API 0.3.0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for method newer than min_api_version, got %+v", issues)
	}
}

func TestDrift_methodRemovedAtMin(t *testing.T) {
	m := minimalValid()
	m.MinAPIVersion = "0.5.0"
	called := []CalledMethod{
		{Name: "removed.thing", File: "src/main.go", Line: 99},
	}
	issues := driftIssues(t, m, called)
	found := false
	for _, i := range issues {
		if i.Severity == SeverityError && strings.Contains(i.Message, "was removed in API 0.5.0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for removed method, got %+v", issues)
	}
}

func TestDrift_methodDeprecatedWarn(t *testing.T) {
	m := minimalValid()
	m.MinAPIVersion = "0.1.0"
	called := []CalledMethod{
		{Name: "old.thing", File: "src/main.go", Line: 12},
	}
	issues := driftIssues(t, m, called)
	found := false
	for _, i := range issues {
		if i.Severity == SeverityWarn && strings.Contains(i.Message, "deprecated as of API 0.4.0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for deprecated method, got %+v", issues)
	}
}

func TestDrift_unknownMethodIsInfo(t *testing.T) {
	m := minimalValid()
	called := []CalledMethod{
		{Name: "plugin.other.helper", File: "src/main.go", Line: 7},
	}
	issues := driftIssues(t, m, called)
	found := false
	for _, i := range issues {
		if i.Severity == SeverityInfo && strings.Contains(i.Message, "plugin.other.helper") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info note for unknown method, got %+v", issues)
	}
}

func TestDrift_dedupesByName(t *testing.T) {
	m := minimalValid()
	called := []CalledMethod{
		{Name: "plugin.other.helper", File: "a.go", Line: 1},
		{Name: "plugin.other.helper", File: "b.go", Line: 2},
		{Name: "plugin.other.helper", File: "c.go", Line: 3},
	}
	issues := driftIssues(t, m, called)
	count := 0
	for _, i := range issues {
		if strings.Contains(i.Message, "plugin.other.helper") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected method to surface once, got %d issues: %+v", count, issues)
	}
}

func TestDrift_nilSpecReturnsNothing(t *testing.T) {
	m := minimalValid()
	m.Privileges = []string{"made-up"}
	if got := CheckDrift(m, nil, nil); got != nil {
		t.Errorf("expected nil with nil spec, got %+v", got)
	}
}

func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.2.0", "0.1.0", false},
		{"1.0.0", "0.99.99", false},
		{"0.99.99", "1.0.0", true},
		{"0.1.0", "0.1.0", false},
		{"", "0.1.0", false},
		{"0.1.0", "", false},
		{"bad", "0.1.0", false},
	}
	for _, c := range cases {
		got := SemverLess(c.a, c.b)
		if got != c.want {
			t.Errorf("SemverLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestParseSpec_realFixture(t *testing.T) {
	spec, err := LoadEmbeddedSpec()
	if err != nil {
		t.Fatalf("LoadEmbeddedSpec: %v", err)
	}
	if spec.APIVersion == "" {
		t.Error("embedded spec missing info.version")
	}
	if len(spec.Methods) == 0 {
		t.Error("embedded spec has zero methods")
	}
	if len(spec.Privileges) == 0 {
		t.Error("embedded spec has zero privileges")
	}
	// Methods we know exist in the current spec. (collection.push died in the
	// unified collections API — put/append are the write verbs now.)
	for _, name := range []string{"collection.get", "collection.put", "events.emit"} {
		if _, ok := spec.Methods[name]; !ok {
			t.Errorf("expected method %q in spec, not found", name)
		}
	}
}
