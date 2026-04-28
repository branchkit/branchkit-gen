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

// PluginManifest is the subset of a BranchKit plugin.json that the
// codegen and validation tools read. Fields not declared here are
// ignored by json.Unmarshal; the validator catches unknown top-level
// keys via RawJSON.
type PluginManifest struct {
	ID                 string                      `json:"id"`
	Name               string                      `json:"name"`
	Version            string                      `json:"version"`
	Description        string                      `json:"description"`
	Author             string                      `json:"author"`
	MinAPIVersion      string                      `json:"min_api_version"`
	Run                string                      `json:"run"`
	ActionPrefix       *string                     `json:"action_prefix"`
	ActionPrefixAccess string                      `json:"action_prefix_access"`
	SettingsTab        *string                     `json:"settings_tab"`
	Implements         PluginImplements            `json:"implements"`
	Capabilities       []string                    `json:"capabilities"`
	DispatchVia        string                      `json:"dispatch_via"`
	DispatchPrefixes   []string                    `json:"dispatch_prefixes"`
	ActionTypes        map[string]ActionTypeSchema `json:"action_types"`
}

// PluginImplements parses the "implements" block. settings_tabs is the
// only well-known key; everything else is a method declaration whose
// presence (regardless of value) signals that the plugin handles that
// JSON-RPC method.
type PluginImplements struct {
	SettingsTabs []SettingsTab
	Methods      map[string]json.RawMessage
}

// SettingsTab is one entry in implements.settings_tabs.
type SettingsTab struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// HasMethod reports whether the plugin implements the given JSON-RPC method.
func (p *PluginImplements) HasMethod(name string) bool {
	_, ok := p.Methods[name]
	return ok
}

// UnmarshalJSON splits the open-ended implements block into the
// settings_tabs list and a generic method map.
func (p *PluginImplements) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Methods = make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		if k == "settings_tabs" {
			if err := json.Unmarshal(v, &p.SettingsTabs); err != nil {
				return fmt.Errorf("implements.settings_tabs: %w", err)
			}
			continue
		}
		p.Methods[k] = v
	}
	return nil
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
	m, _, err := LoadManifestRaw(pluginDir)
	return m, err
}

// LoadManifestRaw also returns the unmarshaled top-level JSON object so
// validators can check for unknown fields without round-tripping through
// the typed struct.
func LoadManifestRaw(pluginDir string) (*PluginManifest, map[string]any, error) {
	path := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, raw, nil
}
