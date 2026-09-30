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

func TestRenderTS_DefaultFieldType(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {Fields: []ActionFieldSchema{{Key: "x", Required: true}}},
		},
	}
	out := RenderTS(m)
	if !strings.Contains(out, "x: string;") {
		t.Errorf("omitted field_type should default to string:\n%s", out)
	}
}

func TestRenderTS_JsonFieldType(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"do": {Fields: []ActionFieldSchema{{Key: "data", FieldType: FieldTypeJson, Required: true}}},
		},
	}
	out := RenderTS(m)
	if !strings.Contains(out, "data: unknown;") {
		t.Errorf("json field should map to unknown:\n%s", out)
	}
}

// Every action gets a typed registrar, as in Go (Handle<Action>) and Python
// (handle_<action>): the action string and params type come from the manifest.
// An action with no fields takes the untyped handler, as in Go.
func TestRenderTS_ActionRegistrar(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"snap":  {Fields: []ActionFieldSchema{{Key: "edge", Required: true}}},
			"reset": {},
		},
	}
	out := RenderTS(m)
	for _, want := range []string{
		`import type { Plugin } from "@branchkitdev/plugin-sdk-ts";`,
		"export function handleSnap(\n  plugin: Plugin,\n  fn: Parameters<typeof plugin.handleAction<SnapParams>>[1],\n): void {\n  plugin.handleAction<SnapParams>(\"wm.snap\", fn);\n}",
		"export function handleReset(\n  plugin: Plugin,\n  fn: Parameters<typeof plugin.handleAction>[1],\n): void {\n  plugin.handleAction(\"wm.reset\", fn);\n}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing:\n%s\nin:\n%s", want, out)
		}
	}
}

// A manifest with no action types imports nothing: an unused import is lint
// noise every consumer inherits.
func TestRenderTS_NoActionsNoImport(t *testing.T) {
	out := RenderTS(&PluginManifest{})
	if strings.Contains(out, "import") {
		t.Errorf("no actions should mean no import:\n%s", out)
	}
}
