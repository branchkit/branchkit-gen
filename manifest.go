package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Privileges         []string                    `json:"privileges"`
	DispatchVia        string                      `json:"dispatch_via"`
	Consumes           Consumes                    `json:"consumes"`
	ActionTypes        map[string]ActionTypeSchema `json:"action_types"`
}

// Consumes is the consumption block. Only the parts this validator
// reasons about are modelled; the rest is checked against the manifest
// schema, which is generated and embedded.
type Consumes struct {
	Collections []ConsumedCollection `json:"collections"`
	Dispatch    []ConsumedDispatch   `json:"dispatch"`
}

// ConsumedCollection is a bare collection name or an object naming the
// fields this plugin reads (and, for a seed source, the grammar targets its
// keys feed). Same untagged shape the platform uses.
type ConsumedCollection struct {
	Name   string
	Fields []string
	Seeds  []string
}

func (c *ConsumedCollection) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		c.Name, c.Fields, c.Seeds = s, nil, nil
		return nil
	}
	var o struct {
		Name   string   `json:"name"`
		Fields []string `json:"fields"`
		Seeds  []string `json:"seeds"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	c.Name, c.Fields, c.Seeds = o.Name, o.Fields, o.Seeds
	return nil
}

// ConsumedDispatch is a bare prefix string or an object naming the action
// types dispatched under it. Same untagged shape the platform uses, so a
// manifest that is valid here is valid there.
type ConsumedDispatch struct {
	Prefix  string
	Actions []string
}

func (c *ConsumedDispatch) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		c.Prefix, c.Actions = s, nil
		return nil
	}
	var o struct {
		Prefix  string   `json:"prefix"`
		Actions []string `json:"actions"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	c.Prefix, c.Actions = o.Prefix, o.Actions
	return nil
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
	// Display is the field's display role (primary, secondary, group,
	// description, payload, summary). Validated against the closed set:
	// publish-time is the STRICT half of the platform's publish-strict /
	// load-tolerant rule for display vocabulary — the host degrades an
	// unknown role to no-role with a warning, so this validator is where
	// typos actually get caught.
	Display string `json:"display"`
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
// resolveCollectionRefs replaces every path-valued `provides.collections`
// entry in a raw manifest object with the contents of the file it names.
//
// The manifest schema allows a collection's declaration to be written either
// inline or as a path relative to the plugin directory — the same shorthand
// `collection_data` has always had for records. A path is a VALUE, not an
// expression: nothing is evaluated, and the file holds the same static
// object that would otherwise sit inline.
//
// Mirrors `resolve_collection_refs` in the actuator, including its
// containment rules. The two must agree: this tool and the runtime read the
// same manifests, and a disagreement about what a declaration says is the
// kind of split that only shows up in production.
func resolveCollectionRefs(pluginDir string, raw map[string]any) error {
	provides, ok := raw["provides"].(map[string]any)
	if !ok {
		return nil
	}
	collections, ok := provides["collections"].(map[string]any)
	if !ok {
		return nil
	}
	base, err := filepath.Abs(pluginDir)
	if err != nil {
		return err
	}
	for name, value := range collections {
		rel, isPath := value.(string)
		if !isPath {
			continue
		}
		if filepath.IsAbs(rel) || strings.Contains(filepath.ToSlash(rel), "../") || rel == ".." {
			return fmt.Errorf("provides.collections.%s: %q must be a relative path inside the plugin directory", name, rel)
		}
		full := filepath.Join(base, filepath.FromSlash(rel))
		if !strings.HasPrefix(full, base+string(filepath.Separator)) {
			return fmt.Errorf("provides.collections.%s: %q resolves outside the plugin directory", name, rel)
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("provides.collections.%s: cannot read %s: %w", name, rel, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(body, &schema); err != nil {
			return fmt.Errorf("provides.collections.%s: %s: %w", name, rel, err)
		}
		collections[name] = schema
	}
	return nil
}

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
	// Pull in any declaration the manifest referenced by path, so every
	// caller downstream sees one fully-resolved object.
	if err := resolveCollectionRefs(pluginDir, raw); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, raw, nil
}
