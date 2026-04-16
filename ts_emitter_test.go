package main

import (
	"strings"
	"testing"
)

func TestRenderTS_EnumLiteralUnion(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"focus": {
				Fields: []ActionFieldSchema{
					{
						Key: "direction", FieldType: FieldTypeEnum, Required: true,
						EnumValues: []string{"left", "right"},
					},
				},
			},
		},
	}
	out := RenderTS(m)
	if !strings.Contains(out, `export type FocusDirection = "left" | "right";`) {
		t.Errorf("missing literal union:\n%s", out)
	}
	if !strings.Contains(out, "direction: FocusDirection;") {
		t.Errorf("missing typed field:\n%s", out)
	}
}

func TestRenderTS_OptionalField(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"layout": {
				Fields: []ActionFieldSchema{
					{Key: "name", FieldType: FieldTypeString, Required: false},
				},
			},
		},
	}
	out := RenderTS(m)
	if !strings.Contains(out, "name?: string;") {
		t.Errorf("optional field should use ?:\n%s", out)
	}
}

func TestRenderTS_IntAndNumberBothMapToNumber(t *testing.T) {
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
	out := RenderTS(m)
	if !strings.Contains(out, "count: number;") {
		t.Errorf("int should map to number:\n%s", out)
	}
	if !strings.Contains(out, "weight: number;") {
		t.Errorf("number should map to number:\n%s", out)
	}
}
