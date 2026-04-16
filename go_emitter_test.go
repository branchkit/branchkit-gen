package main

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func TestRenderGo_StringField(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"snap": {
				Label: "Snap Window",
				Fields: []ActionFieldSchema{
					{Key: "position", FieldType: FieldTypeString, Required: true},
				},
			},
		},
	}
	out := RenderGo(m)
	if !strings.Contains(out, "type SnapParams struct {") {
		t.Errorf("missing SnapParams struct:\n%s", out)
	}
	if !strings.Contains(out, `Position string `+"`json:\"position\"`") {
		t.Errorf("missing typed Position field:\n%s", out)
	}
}

func TestRenderGo_EnumField(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"focus": {
				Fields: []ActionFieldSchema{
					{
						Key: "direction", FieldType: FieldTypeEnum, Required: true,
						EnumValues: []string{"left", "right", "up", "down"},
					},
				},
			},
		},
	}
	out := RenderGo(m)
	if !strings.Contains(out, "type FocusDirection string") {
		t.Errorf("missing enum type:\n%s", out)
	}
	if !strings.Contains(out, `FocusDirectionLeft FocusDirection = "left"`) {
		t.Errorf("missing enum constant:\n%s", out)
	}
	if !strings.Contains(out, "Direction FocusDirection") {
		t.Errorf("missing typed field:\n%s", out)
	}
}

func TestRenderGo_OptionalPointer(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {
				Fields: []ActionFieldSchema{
					{Key: "name", FieldType: FieldTypeString, Required: false},
				},
			},
		},
	}
	out := RenderGo(m)
	if !strings.Contains(out, "Name *string") {
		t.Errorf("optional string should be pointer:\n%s", out)
	}
	if !strings.Contains(out, `"name,omitempty"`) {
		t.Errorf("optional field should have omitempty:\n%s", out)
	}
}

func TestRenderGo_IntVsNumber(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {
				Fields: []ActionFieldSchema{
					{Key: "count", FieldType: FieldTypeInt, Required: true},
					{Key: "weight", FieldType: FieldTypeNumber, Required: true},
				},
			},
		},
	}
	out := RenderGo(m)
	if !strings.Contains(out, "Count int") {
		t.Errorf("int field should be int:\n%s", out)
	}
	if !strings.Contains(out, "Weight float64") {
		t.Errorf("number field should be float64:\n%s", out)
	}
}

func TestRenderGo_JSONImportOnlyWhenNeeded(t *testing.T) {
	simple := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {Fields: []ActionFieldSchema{{Key: "x", FieldType: FieldTypeString, Required: true}}},
		},
	}
	if strings.Contains(RenderGo(simple), "encoding/json") {
		t.Error("simple string field should not trigger json import")
	}

	withJSON := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {Fields: []ActionFieldSchema{{Key: "data", FieldType: FieldTypeJson, Required: true}}},
		},
	}
	if !strings.Contains(RenderGo(withJSON), "encoding/json") {
		t.Error("json field type should trigger json import")
	}
}

func TestRenderGo_AlphabeticalOrder(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"zebra": {}, "alpha": {}, "mango": {},
		},
	}
	out := RenderGo(m)
	a := strings.Index(out, "AlphaParams")
	mg := strings.Index(out, "MangoParams")
	z := strings.Index(out, "ZebraParams")
	if a < 0 || mg < 0 || z < 0 || !(a < mg && mg < z) {
		t.Errorf("expected alphabetical order, got alpha@%d mango@%d zebra@%d", a, mg, z)
	}
}

func TestRenderGo_DefaultFieldType(t *testing.T) {
	// field_type omitted in JSON → FieldType("") → defaults to string.
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {Fields: []ActionFieldSchema{{Key: "x", Required: true}}},
		},
	}
	out := RenderGo(m)
	if !strings.Contains(out, "X string") {
		t.Errorf("omitted field_type should default to string:\n%s", out)
	}
}
