package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed specs/actuator-rpc.json
var embeddedSpec []byte

// Spec is the parsed subset of the public OpenRPC spec the validator
// uses. The spec ships embedded in the binary; users never need to
// download it. Sync from `contracts/` is automated by the parent
// Justfile and committed into branchkit-gen so `go install` users get
// a known-good copy.
type Spec struct {
	APIVersion   string
	Methods      map[string]*SpecMethod
	Capabilities map[string]bool
}

// SpecMethod is the version metadata for one RPC method. Other fields
// (params, result, etc.) aren't relevant to drift detection.
type SpecMethod struct {
	Name         string
	Since        string
	DeprecatedIn string
	RemovedIn    string
}

// rawSpec mirrors the on-disk JSON shape just enough for parsing.
type rawSpec struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Methods []rawSpecMethod `json:"methods"`
	XCaps   []rawSpecCap    `json:"x-branchkit-privileges"`
}

type rawSpecMethod struct {
	Name         string `json:"name"`
	Since        string `json:"x-since"`
	DeprecatedIn string `json:"x-deprecated-in"`
	RemovedIn    string `json:"x-removed-in"`
}

type rawSpecCap struct {
	Name string `json:"name"`
}

// LoadEmbeddedSpec parses the OpenRPC spec compiled into the binary.
// Returns an error only if the embed itself is malformed — a freshly
// built branchkit-gen always has a valid spec.
func LoadEmbeddedSpec() (*Spec, error) {
	return parseSpec(embeddedSpec)
}

func parseSpec(data []byte) (*Spec, error) {
	var raw rawSpec
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse embedded spec: %w", err)
	}
	s := &Spec{
		APIVersion:   raw.Info.Version,
		Methods:      make(map[string]*SpecMethod, len(raw.Methods)),
		Capabilities: make(map[string]bool, len(raw.XCaps)),
	}
	for _, m := range raw.Methods {
		s.Methods[m.Name] = &SpecMethod{
			Name:         m.Name,
			Since:        m.Since,
			DeprecatedIn: m.DeprecatedIn,
			RemovedIn:    m.RemovedIn,
		}
	}
	for _, c := range raw.XCaps {
		s.Capabilities[c.Name] = true
	}
	return s, nil
}

// SemverLess reports whether version a is strictly less than b.
// Both must be strict semver (N.N.N). Malformed input returns false.
func SemverLess(a, b string) bool {
	pa, ok := parseSemver(a)
	if !ok {
		return false
	}
	pb, ok := parseSemver(b)
	if !ok {
		return false
	}
	for i := range 3 {
		if pa[i] < pb[i] {
			return true
		}
		if pa[i] > pb[i] {
			return false
		}
	}
	return false
}

// SemverLessOrEqual reports whether a <= b.
func SemverLessOrEqual(a, b string) bool {
	return a == b || SemverLess(a, b)
}

func parseSemver(s string) ([3]uint, bool) {
	var out [3]uint
	if s == "" {
		return out, false
	}
	idx := 0
	for part := range 3 {
		var n uint
		started := false
		for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
			n = n*10 + uint(s[idx]-'0')
			idx++
			started = true
		}
		if !started {
			return out, false
		}
		out[part] = n
		if part < 2 {
			if idx >= len(s) || s[idx] != '.' {
				return out, false
			}
			idx++
		}
	}
	if idx != len(s) {
		// Trailing characters (e.g. "0.1.0-rc.1") — accept but ignore.
		// Tightening this is a future call.
	}
	return out, true
}
