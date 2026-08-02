package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Severity ranks a validation issue's impact.
type Severity int

const (
	// SeverityError blocks plugin loading. Authors must fix these.
	SeverityError Severity = iota
	// SeverityWarn is logged but the plugin still loads.
	SeverityWarn
	// SeverityInfo is an informational note (e.g. unrecognized method name).
	SeverityInfo
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarn:
		return "warn"
	case SeverityInfo:
		return "info"
	}
	return "unknown"
}

// Issue is a single validation finding for a manifest.
type Issue struct {
	Severity Severity `json:"severity"`
	PluginID string   `json:"plugin_id"`
	Field    string   `json:"field"`
	Message  string   `json:"message"`
}

// MarshalJSON serializes Severity as a string for stable CI consumption.
func (i Issue) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Severity string `json:"severity"`
		PluginID string `json:"plugin_id"`
		Field    string `json:"field"`
		Message  string `json:"message"`
	}{
		Severity: i.Severity.String(),
		PluginID: i.PluginID,
		Field:    i.Field,
		Message:  i.Message,
	})
}

var (
	idRegex           = regexp.MustCompile(`^[a-z0-9-]+$`)
	semverLooseRegex  = regexp.MustCompile(`^\d+\.\d+\.\d+`)
	semverStrictRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	tabKeyRegex       = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

// reservedActionPrefixes are routing verbs the matching engine reserves;
// a plugin can't claim them as its own action_prefix.
var reservedActionPrefixes = map[string]bool{
	"show":  true,
	"open":  true,
	"hide":  true,
	"close": true,
}

// validFieldTypes are the field_type values the codegen and platform
// recognize. Mirrored from manifest.go's FieldType constants.
var validFieldTypes = map[FieldType]bool{
	FieldTypeString:      true,
	FieldTypeInt:         true,
	FieldTypeNumber:      true,
	FieldTypeBoolean:     true,
	FieldTypeStringArray: true,
	FieldTypeEnum:        true,
	FieldTypeObject:      true,
	FieldTypeJson:        true,
}

// validDisplayRoles are the field display roles the platform recognizes
// (the FieldDisplay enum). Publish-time validation is STRICT here on
// purpose — the platform's rule for display vocabulary is publish-strict /
// load-tolerant: the host degrades an unknown role to no-role with a
// warning (display roles are display-only, so a plugin built against a
// newer SDK must still load on an older host), which means this validator
// is where a typo actually stops the author. A drift test pins this list
// to the FieldDisplay enum in specs/manifest-schema.json.
var validDisplayRoles = map[string]bool{
	"primary":     true,
	"secondary":   true,
	"group":       true,
	"description": true,
	"payload":     true,
	"summary":     true,
}

// knownTopLevelFields lists every top-level plugin.json key the platform
// understands. Anything else gets an info-level note (typo guard).
var knownTopLevelFields = map[string]bool{
	"$schema":              true,
	"id":                   true,
	"name":                 true,
	"version":              true,
	"description":          true,
	"author":               true,
	"implements":           true,
	"run":                  true,
	"min_api_version":      true,
	"settings_tab":         true,
	"collection_data":      true,
	"depends_on":           true,
	"action_prefix":        true,
	"action_prefix_access": true,
	"dispatch_via":         true,
	"dispatch_prefixes":    true,
	"action_types":         true,
	"hud_targets":          true,
	"default_footer":       true,
	"capabilities":         true,
	"provides":             true,
	"consumes":             true,
	"bridge":               true,
	"network":              true,
	"hud_windows":          true,
}

// Validate runs single-manifest checks against m. raw is the original
// top-level object (used for unknown-field detection); pass nil to skip
// that check.
func Validate(m *PluginManifest, raw map[string]any) []Issue {
	var issues []Issue
	id := m.ID

	add := func(sev Severity, field, msg string) {
		issues = append(issues, Issue{Severity: sev, PluginID: id, Field: field, Message: msg})
	}

	// id format
	if id == "" {
		add(SeverityError, "id", "id must not be empty")
	} else if !idRegex.MatchString(id) {
		add(SeverityError, "id", fmt.Sprintf(
			"id %q must match [a-z0-9-]+ (lowercase, digits, hyphens only)", id))
	}

	// name presence
	if m.Name == "" {
		add(SeverityWarn, "name", "name is empty — consider setting a display name")
	}

	// version semver (loose)
	if m.Version == "" {
		add(SeverityInfo, "version", "version is empty — consider adding a semver version")
	} else if !semverLooseRegex.MatchString(m.Version) {
		add(SeverityWarn, "version", fmt.Sprintf(
			"version %q does not look like semver (expected N.N.N)", m.Version))
	}

	// min_api_version (required, strict semver)
	if m.MinAPIVersion == "" {
		add(SeverityError, "min_api_version",
			"min_api_version is required — declare the minimum API version this plugin targets (e.g. \"0.1.0\")")
	} else if !semverStrictRegex.MatchString(m.MinAPIVersion) {
		add(SeverityError, "min_api_version", fmt.Sprintf(
			"min_api_version %q is not valid semver (expected N.N.N)", m.MinAPIVersion))
	}

	// action_prefix format and reserved-word collision
	if m.ActionPrefix != nil {
		prefix := *m.ActionPrefix
		if !idRegex.MatchString(prefix) {
			add(SeverityError, "action_prefix", fmt.Sprintf(
				"action_prefix %q must match [a-z0-9-]+ (lowercase, digits, hyphens only)", prefix))
		}
		if reservedActionPrefixes[prefix] {
			add(SeverityError, "action_prefix", fmt.Sprintf(
				"action_prefix %q is a reserved routing verb", prefix))
		}
	}

	// action_prefix required when on_action is implemented
	if m.Implements.HasMethod("on_action") && m.ActionPrefix == nil {
		add(SeverityError, "action_prefix",
			"action_prefix is required when implements.on_action is declared — "+
				"the platform needs a prefix to route dispatch calls to this plugin")
	}

	// action_prefix_access without action_prefix is meaningless
	if m.ActionPrefix == nil && m.ActionPrefixAccess != "" && m.ActionPrefixAccess != "open" {
		add(SeverityWarn, "action_prefix_access",
			"action_prefix_access has no effect without action_prefix")
	}

	// dispatch_via requires the dispatch capability
	if m.DispatchVia != "" && !slices.Contains(m.Capabilities, "dispatch") {
		add(SeverityError, "dispatch_via",
			"dispatch_via requires 'dispatch' in capabilities")
	}

	// dispatch_via "direct" requires non-empty dispatch_prefixes
	if m.DispatchVia == "direct" && len(m.DispatchPrefixes) == 0 {
		add(SeverityError, "dispatch_prefixes",
			"dispatch_via 'direct' requires non-empty dispatch_prefixes")
	}

	// dispatch_prefixes only used when dispatch_via is "direct"
	if len(m.DispatchPrefixes) > 0 && m.DispatchVia != "direct" {
		add(SeverityWarn, "dispatch_prefixes",
			"dispatch_prefixes is only used when dispatch_via is 'direct'")
	}

	// settings_tab (singular) is deprecated
	if m.SettingsTab != nil {
		add(SeverityWarn, "settings_tab",
			"settings_tab (singular) is deprecated — use implements.settings_tabs instead")
	}

	// settings_tabs[].key charset (security-relevant — flows into URLs and HTML attrs)
	for i, tab := range m.Implements.SettingsTabs {
		if !tabKeyRegex.MatchString(tab.Key) {
			add(SeverityError,
				fmt.Sprintf("implements.settings_tabs[%d].key", i),
				fmt.Sprintf(
					"settings_tabs key %q must match [a-z0-9_-]+ (lowercase, digits, hyphens, underscores). "+
						"Other characters are rejected because the key flows into HTML attributes and URL paths.",
					tab.Key))
		}
	}

	// action_types: well-formed field types and enum_values presence
	for actionName, schema := range m.ActionTypes {
		validateActionType(actionName, schema, add)
	}

	// provides.collections field display roles. Collections aren't modeled
	// as typed structs here, so walk the raw JSON. Same strict posture as
	// action_types' display check — see validDisplayRoles.
	validateCollectionDisplayRoles(raw, add)

	// unknown top-level fields (range over nil map is a no-op)
	for key := range raw {
		if !knownTopLevelFields[key] {
			add(SeverityInfo, key, fmt.Sprintf(
				"unrecognized top-level field %q — this version does not understand it", key))
		}
	}

	return issues
}

// validateCollectionDisplayRoles walks provides.collections.*.fields[*]
// in the raw manifest (recursing into nested object fields) and errors on
// any `display` value outside validDisplayRoles.
func validateCollectionDisplayRoles(raw map[string]any, add func(Severity, string, string)) {
	provides, _ := raw["provides"].(map[string]any)
	collections, _ := provides["collections"].(map[string]any)
	for name, decl := range collections {
		declMap, _ := decl.(map[string]any)
		fields, _ := declMap["fields"].([]any)
		walkCollectionFields(fmt.Sprintf("provides.collections.%s", name), fields, add)
	}
}

func walkCollectionFields(path string, fields []any, add func(Severity, string, string)) {
	for i, field := range fields {
		fieldMap, _ := field.(map[string]any)
		if fieldMap == nil {
			continue
		}
		fieldPath := fmt.Sprintf("%s.fields[%d]", path, i)
		if display, ok := fieldMap["display"]; ok && display != nil {
			role, isString := display.(string)
			if !isString || !validDisplayRoles[role] {
				add(SeverityError, fieldPath+".display", fmt.Sprintf(
					"display role %v is not recognized (valid: primary, secondary, group, description, payload, summary). "+
						"Hosts degrade unknown roles to no-role at load, so an unrecognized value here would silently render nothing",
					display))
			}
		}
		if nested, ok := fieldMap["fields"].([]any); ok {
			walkCollectionFields(fieldPath, nested, add)
		}
	}
}

func validateActionType(actionName string, schema ActionTypeSchema, add func(Severity, string, string)) {
	for i, f := range schema.Fields {
		fieldPath := fmt.Sprintf("action_types.%s.fields[%d]", actionName, i)
		validateActionField(fieldPath, f, add)
	}
}

func validateActionField(fieldPath string, f ActionFieldSchema, add func(Severity, string, string)) {
	ft := f.EffectiveFieldType()
	if !validFieldTypes[ft] {
		add(SeverityError, fieldPath+".field_type", fmt.Sprintf(
			"field_type %q is not a recognized type (valid: string, int, number, boolean, string[], enum, object, json)",
			f.FieldType))
	}
	if ft == FieldTypeEnum && len(f.EnumValues) == 0 {
		add(SeverityError, fieldPath+".enum_values",
			"field_type 'enum' requires non-empty enum_values")
	}
	if ft != FieldTypeEnum && len(f.EnumValues) > 0 {
		add(SeverityWarn, fieldPath+".enum_values",
			"enum_values is only meaningful when field_type is 'enum'")
	}
	if f.Display != "" && !validDisplayRoles[f.Display] {
		add(SeverityError, fieldPath+".display", fmt.Sprintf(
			"display role %q is not recognized (valid: primary, secondary, group, description, payload, summary). "+
				"Hosts degrade unknown roles to no-role at load, so an unrecognized value here would silently render nothing",
			f.Display))
	}
	for i, sub := range f.Fields {
		validateActionField(fmt.Sprintf("%s.fields[%d]", fieldPath, i), sub, add)
	}
}

// HasErrors returns true if any issue is severity error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// CountBySeverity returns counts of errors, warnings, infos.
func CountBySeverity(issues []Issue) (errors, warns, infos int) {
	for _, i := range issues {
		switch i.Severity {
		case SeverityError:
			errors++
		case SeverityWarn:
			warns++
		case SeverityInfo:
			infos++
		}
	}
	return
}

// formatHuman renders issues as terminal-friendly lines.
func formatHuman(issues []Issue) string {
	if len(issues) == 0 {
		return "  No issues found.\n"
	}
	var b strings.Builder
	for _, i := range issues {
		var symbol string
		switch i.Severity {
		case SeverityError:
			symbol = "✗"
		case SeverityWarn:
			symbol = "⚠"
		case SeverityInfo:
			symbol = "·"
		}
		fmt.Fprintf(&b, "  %s [%s] %s: %s — %s\n", symbol, i.Severity, i.PluginID, i.Field, i.Message)
	}
	errors, warns, infos := CountBySeverity(issues)
	b.WriteString("\n")
	if errors > 0 {
		b.WriteString("  " + strconv.Itoa(errors) + " error(s)\n")
	}
	if warns > 0 {
		b.WriteString("  " + strconv.Itoa(warns) + " warning(s)\n")
	}
	if infos > 0 {
		b.WriteString("  " + strconv.Itoa(infos) + " info(s)\n")
	}
	return b.String()
}
