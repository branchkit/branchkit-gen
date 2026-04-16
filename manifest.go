package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FieldType is the declared type of an action field. Deserializes directly
// from the "field_type" JSON key in plugin.json. Unknown values unmarshal as
// non-empty strings that don't match any constant — the generator warns and
// falls through to json.RawMessage / unknown.
type FieldType string

const (
	FieldTypeString      FieldType = "string"
	FieldTypeInt         FieldType = "int"
	FieldTypeNumber      FieldType = "number"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeStringArray FieldType = "string[]"
	FieldTypeEnum        FieldType = "enum"
	FieldTypeObject      FieldType = "object"
	FieldTypeJson        FieldType = "json"
)

// NeedsJSONImport returns true for field types that emit json.RawMessage
// in Go output, requiring an encoding/json import.
func (ft FieldType) NeedsJSONImport() bool {
	return ft == FieldTypeObject || ft == FieldTypeJson
}

// PluginManifest is the minimal subset of a BranchKit plugin.json that the
// codegen tool reads. All other fields (provides, consumes, implements,
// hud_windows, etc.) are silently ignored by json.Unmarshal.
type PluginManifest struct {
	ID           string                      `json:"id"`
	ActionPrefix *string                     `json:"action_prefix"`
	ActionTypes  map[string]ActionTypeSchema `json:"action_types"`
}

// ActionTypeSchema is a single entry in the action_types map.
type ActionTypeSchema struct {
	Label  string              `json:"label"`
	Fields []ActionFieldSchema `json:"fields"`
}

// ActionFieldSchema is one field within an action type's params.
type ActionFieldSchema struct {
	Key         string              `json:"key"`
	Label       string              `json:"label"`
	Placeholder string              `json:"placeholder"`
	FieldType   FieldType           `json:"field_type"`
	Required    bool                `json:"required"`
	EnumValues  []string            `json:"enum_values"`
	Fields      []ActionFieldSchema `json:"fields"`
}

// EffectiveFieldType returns the field type, defaulting to FieldTypeString
// when the manifest omits the field (Go deserializes absent string as "").
func (f *ActionFieldSchema) EffectiveFieldType() FieldType {
	if f.FieldType == "" {
		return FieldTypeString
	}
	return f.FieldType
}

// LoadManifest reads and parses a plugin.json from the given plugin directory.
func LoadManifest(pluginDir string) (*PluginManifest, error) {
	path := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}
