package main

import (
	"strings"
	"unicode"
)

// fieldAcronyms are segments that receive full-uppercase treatment when
// converting snake_case JSON keys to Go identifiers. Matches the actuator's
// go_types::FIELD_ACRONYMS set so generated output is byte-identical.
var fieldAcronyms = map[string]bool{
	"id": true, "ip": true, "url": true, "html": true,
	"css": true, "js": true, "ui": true, "os": true,
	"api": true, "uid": true,
}

// GoIdentifier converts a snake_case / kebab-case / dotted key to a Go
// PascalCase identifier, uppercasing segments that match fieldAcronyms.
//
// Examples:
//
//	"bundle_id"        → "BundleID"
//	"active_window_id" → "ActiveWindowID"
//	"source_url"       → "SourceURL"
//	"move_to"          → "MoveTo"
//	"direction"        → "Direction"
func GoIdentifier(s string) string {
	var b strings.Builder
	for _, part := range splitIdentifier(s) {
		if fieldAcronyms[strings.ToLower(part)] {
			b.WriteString(strings.ToUpper(part))
		} else {
			b.WriteString(CapitalizeFirst(part))
		}
	}
	return b.String()
}

// CapitalizeFirst uppercases the first rune and lowercases the rest.
func CapitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

// splitIdentifier breaks a string on '_', '-', and '.' separators,
// filtering out empty segments.
func splitIdentifier(s string) []string {
	f := func(r rune) bool { return r == '_' || r == '-' || r == '.' }
	parts := strings.FieldsFunc(s, f)
	return parts
}
