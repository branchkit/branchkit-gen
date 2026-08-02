package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// minimalValid is a manifest that should pass validation cleanly. Tests
// build on this base, mutating one field at a time.
func minimalValid() *PluginManifest {
	prefix := "demo"
	return &PluginManifest{
		ID:            "demo-plugin",
		Name:          "Demo Plugin",
		Version:       "0.1.0",
		MinAPIVersion: "0.1.0",
		ActionPrefix:  &prefix,
		Implements: PluginImplements{
			Methods: map[string]json.RawMessage{
				"on_action": json.RawMessage("true"),
			},
		},
	}
}

func issuesByField(issues []Issue) map[string]Severity {
	out := map[string]Severity{}
	for _, i := range issues {
		out[i.Field] = i.Severity
	}
	return out
}

func TestValidate_minimalValid(t *testing.T) {
	got := Validate(minimalValid(), nil)
	if HasErrors(got) {
		t.Fatalf("minimal valid manifest produced errors: %+v", got)
	}
}

func TestValidate_idFormat(t *testing.T) {
	cases := []struct {
		id       string
		wantSev  Severity
		wantHave bool
	}{
		{"demo-plugin", 0, false},
		{"", SeverityError, true},
		{"Demo", SeverityError, true},
		{"demo_plugin", SeverityError, true},
		{"demo plugin", SeverityError, true},
	}
	for _, c := range cases {
		m := minimalValid()
		m.ID = c.id
		got := Validate(m, nil)
		idIssue := false
		var sev Severity
		for _, i := range got {
			if i.Field == "id" {
				idIssue = true
				sev = i.Severity
			}
		}
		if idIssue != c.wantHave {
			t.Errorf("id=%q: got idIssue=%v, want %v (issues=%+v)", c.id, idIssue, c.wantHave, got)
		}
		if c.wantHave && sev != c.wantSev {
			t.Errorf("id=%q: severity=%v, want %v", c.id, sev, c.wantSev)
		}
	}
}

func TestValidate_minAPIVersion(t *testing.T) {
	cases := []struct {
		val      string
		wantHave bool
	}{
		{"", true}, // required — empty is an error
		{"0.1.0", false},
		{"1.2.3", false},
		{"0.1", true},       // not strict semver
		{"v0.1.0", true},    // not strict semver
		{"latest", true},    // not strict semver
		{"0.0.0", false},    // valid semver, even if unusual
		{"99.99.99", false}, // valid semver
	}
	for _, c := range cases {
		m := minimalValid()
		m.MinAPIVersion = c.val
		got := Validate(m, nil)
		have := false
		for _, i := range got {
			if i.Field == "min_api_version" && i.Severity == SeverityError {
				have = true
			}
		}
		if have != c.wantHave {
			t.Errorf("min_api_version=%q: got error=%v, want %v", c.val, have, c.wantHave)
		}
	}
}

func TestValidate_minAPIVersionRequiredMessage(t *testing.T) {
	m := minimalValid()
	m.MinAPIVersion = ""
	got := Validate(m, nil)
	found := false
	for _, i := range got {
		if i.Field == "min_api_version" && i.Severity == SeverityError &&
			strings.Contains(i.Message, "required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'required' in error message for missing min_api_version, got: %+v", got)
	}
}

func TestValidate_minAPIVersionFormatMessage(t *testing.T) {
	m := minimalValid()
	m.MinAPIVersion = "0.1"
	got := Validate(m, nil)
	found := false
	for _, i := range got {
		if i.Field == "min_api_version" && i.Severity == SeverityError &&
			strings.Contains(i.Message, "not valid semver") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'not valid semver' in error message for bad format, got: %+v", got)
	}
}

func TestValidate_actionPrefixReserved(t *testing.T) {
	for _, prefix := range []string{"show", "open", "hide", "close"} {
		m := minimalValid()
		p := prefix
		m.ActionPrefix = &p
		got := Validate(m, nil)
		found := false
		for _, i := range got {
			if i.Field == "action_prefix" && i.Severity == SeverityError &&
				strings.Contains(i.Message, "reserved") {
				found = true
			}
		}
		if !found {
			t.Errorf("action_prefix=%q: expected reserved-word error, issues=%+v", prefix, got)
		}
	}
}

func TestValidate_onActionRequiresPrefix(t *testing.T) {
	m := minimalValid()
	m.ActionPrefix = nil
	got := Validate(m, nil)
	if !HasErrors(got) {
		t.Fatalf("expected error for on_action without prefix, got: %+v", got)
	}
}

func TestValidate_settingsTabKey(t *testing.T) {
	cases := []struct {
		key      string
		wantHave bool
	}{
		{"commands", false},
		{"key-with-hyphen", false},
		{"key_with_underscore", false},
		{"Bad Key", true},
		{`"><script>`, true},
		{"UPPERCASE", true},
	}
	for _, c := range cases {
		m := minimalValid()
		m.Implements.SettingsTabs = []SettingsTab{{Key: c.key, Label: "X"}}
		got := Validate(m, nil)
		have := false
		for _, i := range got {
			if strings.HasPrefix(i.Field, "implements.settings_tabs") && i.Severity == SeverityError {
				have = true
			}
		}
		if have != c.wantHave {
			t.Errorf("settings_tabs key=%q: got error=%v, want %v", c.key, have, c.wantHave)
		}
	}
}

func TestValidate_dispatchVia(t *testing.T) {
	m := minimalValid()
	m.DispatchVia = "direct"
	got := Validate(m, nil)
	fields := issuesByField(got)
	if fields["dispatch_via"] != SeverityError {
		t.Errorf("expected dispatch_via error without 'dispatch' capability, got: %+v", got)
	}
	if fields["dispatch_prefixes"] != SeverityError {
		t.Errorf("expected dispatch_prefixes error for direct mode without prefixes, got: %+v", got)
	}

	m2 := minimalValid()
	m2.Capabilities = []string{"dispatch"}
	m2.DispatchPrefixes = []string{"foo."}
	got2 := Validate(m2, nil)
	if HasErrors(got2) {
		t.Errorf("dispatch_prefixes set without dispatch_via='direct' should warn, not error: %+v", got2)
	}
}

func TestValidate_actionTypes(t *testing.T) {
	t.Run("invalid field type", func(t *testing.T) {
		m := minimalValid()
		m.ActionTypes = map[string]ActionTypeSchema{
			"snap": {
				Fields: []ActionFieldSchema{
					{Key: "thing", FieldType: "bogus"},
				},
			},
		}
		got := Validate(m, nil)
		if !HasErrors(got) {
			t.Errorf("expected error for invalid field_type, got: %+v", got)
		}
	})

	t.Run("enum without enum_values", func(t *testing.T) {
		m := minimalValid()
		m.ActionTypes = map[string]ActionTypeSchema{
			"snap": {
				Fields: []ActionFieldSchema{
					{Key: "direction", FieldType: FieldTypeEnum},
				},
			},
		}
		got := Validate(m, nil)
		if !HasErrors(got) {
			t.Errorf("expected error for enum without enum_values, got: %+v", got)
		}
	})

	t.Run("enum_values on non-enum", func(t *testing.T) {
		m := minimalValid()
		m.ActionTypes = map[string]ActionTypeSchema{
			"snap": {
				Fields: []ActionFieldSchema{
					{Key: "direction", FieldType: FieldTypeString, EnumValues: []string{"left", "right"}},
				},
			},
		}
		got := Validate(m, nil)
		errors, _, _ := CountBySeverity(got)
		if errors > 0 {
			t.Errorf("enum_values on non-enum should warn, not error: %+v", got)
		}
	})

	t.Run("nested object field", func(t *testing.T) {
		m := minimalValid()
		m.ActionTypes = map[string]ActionTypeSchema{
			"toggle": {
				Fields: []ActionFieldSchema{
					{Key: "config", FieldType: FieldTypeObject, Fields: []ActionFieldSchema{
						{Key: "nested", FieldType: "bogus"},
					}},
				},
			},
		}
		got := Validate(m, nil)
		if !HasErrors(got) {
			t.Errorf("expected error for nested invalid field_type, got: %+v", got)
		}
	})
}

func TestValidate_unknownTopLevel(t *testing.T) {
	m := minimalValid()
	raw := map[string]any{
		"id":            "demo-plugin",
		"name":          "Demo",
		"version":       "0.1.0",
		"made_up_field": "oops",
		"action_prefix": "demo",
	}
	got := Validate(m, raw)
	found := false
	for _, i := range got {
		if i.Field == "made_up_field" && i.Severity == SeverityInfo {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info-level note for made_up_field, got: %+v", got)
	}
}

func TestValidate_settingsTabSingularDeprecated(t *testing.T) {
	m := minimalValid()
	tab := "old-style"
	m.SettingsTab = &tab
	got := Validate(m, nil)
	found := false
	for _, i := range got {
		if i.Field == "settings_tab" && i.Severity == SeverityWarn {
			found = true
		}
	}
	if !found {
		t.Errorf("expected deprecated warning for settings_tab, got: %+v", got)
	}
}

func TestIssue_MarshalJSON(t *testing.T) {
	i := Issue{Severity: SeverityError, PluginID: "demo", Field: "id", Message: "bad"}
	b, err := json.Marshal(i)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"severity":"error","plugin_id":"demo","field":"id","message":"bad"}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

// Publish-time is the STRICT half of the platform's publish-strict /
// load-tolerant rule for display vocabulary: hosts degrade an unknown
// display role to no-role (with a warning) so newer-SDK plugins still
// load, which makes this validator the place a typo actually stops the
// author.
func TestValidate_unknownDisplayRoleInActionTypesIsError(t *testing.T) {
	m := minimalValid()
	m.ActionTypes = map[string]ActionTypeSchema{
		"click": {Fields: []ActionFieldSchema{
			{Key: "selector", Display: "sparkle"},
		}},
	}
	got := Validate(m, nil)
	found := false
	for _, i := range got {
		if i.Severity == SeverityError && strings.Contains(i.Field, "click") &&
			strings.Contains(i.Message, "sparkle") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for unknown display role, got: %+v", got)
	}
}

func TestValidate_knownDisplayRolesInActionTypesPass(t *testing.T) {
	m := minimalValid()
	m.ActionTypes = map[string]ActionTypeSchema{
		"click": {Fields: []ActionFieldSchema{
			{Key: "selector", Display: "primary"},
			{Key: "note", Display: "secondary"},
			{Key: "kind", Display: "group"},
			{Key: "id", Display: "payload"},
			{Key: "plain"}, // no role is fine
		}},
	}
	got := Validate(m, nil)
	if HasErrors(got) {
		t.Errorf("known display roles should not error, got: %+v", got)
	}
}

func TestValidate_unknownDisplayRoleInCollectionsIsError(t *testing.T) {
	m := minimalValid()
	raw := map[string]any{
		"provides": map[string]any{
			"collections": map[string]any{
				"demo_rows": map[string]any{
					"fields": []any{
						map[string]any{"key": "spoken", "display": "primary"},
						map[string]any{"key": "kind", "display": "hologram"},
					},
				},
			},
		},
	}
	got := Validate(m, raw)
	found := false
	for _, i := range got {
		if i.Severity == SeverityError && strings.Contains(i.Field, "demo_rows") &&
			strings.Contains(i.Message, "hologram") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for unknown collection display role, got: %+v", got)
	}
}

func TestValidate_nestedCollectionFieldDisplayChecked(t *testing.T) {
	m := minimalValid()
	raw := map[string]any{
		"provides": map[string]any{
			"collections": map[string]any{
				"demo_rows": map[string]any{
					"fields": []any{
						map[string]any{"key": "outer", "fields": []any{
							map[string]any{"key": "inner", "display": "bogus"},
						}},
					},
				},
			},
		},
	}
	got := Validate(m, raw)
	found := false
	for _, i := range got {
		if i.Severity == SeverityError && strings.Contains(i.Message, "bogus") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for nested unknown display role, got: %+v", got)
	}
}

// validDisplayRoles is a hand list; this drift test pins it to the
// FieldDisplay enum in the synced manifest schema so a new role added
// upstream (and synced via `just contracts`) fails here until the list
// is updated.
func TestValidDisplayRoles_matchesManifestSchema(t *testing.T) {
	data, err := os.ReadFile("specs/manifest-schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			OneOf []struct {
				Const string `json:"const"`
			} `json:"oneOf"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	fd, ok := schema.Defs["FieldDisplay"]
	if !ok {
		t.Fatal("FieldDisplay not found in manifest schema $defs")
	}
	fromSchema := map[string]bool{}
	for _, v := range fd.OneOf {
		if v.Const != "" {
			fromSchema[v.Const] = true
		}
	}
	if len(fromSchema) == 0 {
		t.Fatal("no FieldDisplay consts extracted from schema")
	}
	for role := range fromSchema {
		if !validDisplayRoles[role] {
			t.Errorf("schema role %q missing from validDisplayRoles", role)
		}
	}
	for role := range validDisplayRoles {
		if !fromSchema[role] {
			t.Errorf("validDisplayRoles has %q not present in schema", role)
		}
	}
}
