package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// schemaFieldTypes reads every FieldType value the synced manifest schema
// accepts: the plain enum and each documented const.
func schemaFieldTypes(t *testing.T) []FieldType {
	t.Helper()
	data, err := os.ReadFile("specs/manifest-schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			OneOf []struct {
				Const string   `json:"const"`
				Enum  []string `json:"enum"`
			} `json:"oneOf"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	ft, ok := schema.Defs["FieldType"]
	if !ok {
		t.Fatal("FieldType not found in manifest schema $defs")
	}
	var out []FieldType
	for _, v := range ft.OneOf {
		if v.Const != "" {
			out = append(out, FieldType(v.Const))
		}
		for _, e := range v.Enum {
			out = append(out, FieldType(e))
		}
	}
	if len(out) == 0 {
		t.Fatal("no FieldType values extracted from schema")
	}
	return out
}

// validFieldTypes is a hand list; this pins it to the schema, so a field type
// added upstream fails here until the validator (and the emitters, below)
// know it.
func TestValidFieldTypes_matchesManifestSchema(t *testing.T) {
	fromSchema := map[FieldType]bool{}
	for _, ft := range schemaFieldTypes(t) {
		fromSchema[ft] = true
		if !validFieldTypes[ft] {
			t.Errorf("schema field_type %q missing from validFieldTypes", ft)
		}
	}
	for ft := range validFieldTypes {
		if !fromSchema[ft] {
			t.Errorf("validFieldTypes has %q not present in schema", ft)
		}
	}
}

// Only the two declared escape hatches, object and json, may emit a dynamic
// type. Any other field type the schema accepts must come out typed in all
// three languages; an unhandled one falls through to raw JSON silently.
func TestEveryFieldTypeEmitsTyped(t *testing.T) {
	dynamic := map[FieldType]bool{FieldTypeObject: true, FieldTypeJson: true}
	for _, ft := range schemaFieldTypes(t) {
		if dynamic[ft] {
			continue
		}
		f := ActionFieldSchema{Key: "x", FieldType: ft, Required: true, EnumValues: []string{"a"}}
		if got := goFieldType("Do", &f); got == "json.RawMessage" {
			t.Errorf("Go: field_type %q emits %s", ft, got)
		}
		if got := tsFieldType("Do", &f); got == "unknown" {
			t.Errorf("TS: field_type %q emits %s", ft, got)
		}
		if got := pyFieldType("Do", &f); got == "Any" {
			t.Errorf("Python: field_type %q emits %s", ft, got)
		}
	}
}

// secret_ref holds a NAME in the secret store, so it is a string everywhere.
func TestSecretRefIsAString(t *testing.T) {
	m := &PluginManifest{
		ActionPrefix: ptr("p"),
		ActionTypes: map[string]ActionTypeSchema{
			"call": {Fields: []ActionFieldSchema{
				{Key: "token", FieldType: FieldTypeSecretRef, Required: true},
				{Key: "backup", FieldType: FieldTypeSecretRef},
			}},
		},
	}
	goOut := RenderGo(m)
	for _, want := range []string{"Token string `json:\"token\"`", "Backup *string `json:\"backup,omitempty\"`"} {
		if !strings.Contains(strings.Join(strings.Fields(goOut), " "), want) {
			t.Errorf("Go: missing %q:\n%s", want, goOut)
		}
	}
	if strings.Contains(goOut, "encoding/json") {
		t.Errorf("Go: secret_ref should not need encoding/json:\n%s", goOut)
	}
	tsOut := RenderTS(m)
	for _, want := range []string{"token: string;", "backup?: string;"} {
		if !strings.Contains(tsOut, want) {
			t.Errorf("TS: missing %q:\n%s", want, tsOut)
		}
	}
	pyOut := RenderPy(m)
	for _, want := range []string{`"token": str,`, `"backup": NotRequired[str],`} {
		if !strings.Contains(pyOut, want) {
			t.Errorf("Python: missing %q:\n%s", want, pyOut)
		}
	}
	if strings.Contains(pyOut, "Any") {
		t.Errorf("Python: secret_ref should not import Any:\n%s", pyOut)
	}

	v := minimalValid()
	v.ActionTypes = m.ActionTypes
	if got := Validate(v, nil); HasErrors(got) {
		t.Errorf("secret_ref should validate: %+v", got)
	}
}
