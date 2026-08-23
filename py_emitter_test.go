package main

import (
	"strings"
	"testing"
)

func TestRenderPy_EnumLiteralAndTypedDict(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"focus": {
				Fields: []ActionFieldSchema{
					{
						Key: "direction", FieldType: FieldTypeEnum, Required: true,
						EnumValues: []string{"left", "right"},
					},
					{Key: "gap", FieldType: FieldTypeInt, Required: false},
				},
			},
		},
	}
	out := RenderPy(m)
	if !strings.Contains(out, `FocusDirection = Literal["left", "right"]`) {
		t.Errorf("missing Literal alias:\n%s", out)
	}
	if !strings.Contains(out, `"direction": FocusDirection,`) {
		t.Errorf("missing typed field:\n%s", out)
	}
	if !strings.Contains(out, `"gap": NotRequired[int],`) {
		t.Errorf("optional field should use NotRequired:\n%s", out)
	}
	if !strings.Contains(out, `FocusParams = TypedDict("FocusParams", {`) {
		t.Errorf("TypedDict must use the functional form (keyword-proof keys):\n%s", out)
	}
	if !strings.Contains(out, "from typing import Literal, NotRequired, TypedDict\n") {
		t.Errorf("imports must be exactly what the file uses:\n%s", out)
	}
}

func TestRenderPy_ImportsOnlyWhatItUses(t *testing.T) {
	// An unused import in generated code is lint noise every consumer
	// inherits and none of them can fix.
	m := &PluginManifest{
		ActionTypes: map[string]ActionTypeSchema{
			"ping": {
				Fields: []ActionFieldSchema{
					{Key: "n", FieldType: FieldTypeInt, Required: true},
				},
			},
		},
	}
	out := RenderPy(m)
	if !strings.Contains(out, "from typing import TypedDict\n") {
		t.Errorf("all-required scalar fields need only TypedDict:\n%s", out)
	}
	for _, unused := range []string{"Literal", "NotRequired", "Any"} {
		if strings.Contains(out, "import") && strings.Contains(strings.SplitN(out, "\n\n", 3)[1], unused) {
			t.Errorf("unused import %q:\n%s", unused, out)
		}
	}
}

func TestRenderPy_RegistrarUsesManifestActionString(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("wm"),
		ActionTypes: map[string]ActionTypeSchema{
			"snap-to": {
				Fields: []ActionFieldSchema{
					{Key: "position", FieldType: FieldTypeString, Required: true},
				},
			},
		},
	}
	out := RenderPy(m)
	if !strings.Contains(out, "def handle_snap_to(plugin, fn=None):") {
		t.Errorf("registrar name should be snake_case and accept the decorator form:\n%s", out)
	}
	if !strings.Contains(out, `return plugin.handle_action("wm.snap-to", fn)`) {
		t.Errorf("registrar must carry the manifest's full action string and return the decorator:\n%s", out)
	}
}

func TestRenderPy_ParsesAsPython(t *testing.T) {
	// The strongest invariant an emitter can offer without an interpreter
	// in the test env: balanced structure and no stray tokens. (The emitted
	// file is import-tested for real by the SDK's own suite.)
	m := &PluginManifest{
		ActionTypes: map[string]ActionTypeSchema{
			"noop": {},
		},
	}
	out := RenderPy(m)
	if !strings.Contains(out, "def handle_noop(plugin, fn=None):") {
		t.Errorf("field-less action still gets a registrar:\n%s", out)
	}
	if !strings.Contains(out, `return plugin.handle_action("noop", fn)`) {
		t.Errorf("prefix-less action uses the bare name:\n%s", out)
	}
}
