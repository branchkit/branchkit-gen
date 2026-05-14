package main

import (
	"fmt"
	"slices"
	"sort"
)

// CheckDrift validates the manifest and source-analysis result against
// the bundled OpenRPC spec. Issues are appended to the result list.
//
// Drift checks live separately from Validate() because they need the
// embedded spec and (optionally) static-analysis output, both of which
// the core Validate() pass deliberately doesn't depend on.
func CheckDrift(m *PluginManifest, called []CalledMethod, spec *Spec) []Issue {
	if spec == nil {
		return nil
	}
	var issues []Issue
	id := m.ID
	add := func(sev Severity, field, msg string) {
		issues = append(issues, Issue{Severity: sev, PluginID: id, Field: field, Message: msg})
	}

	// 1. Capabilities the plugin declares must exist in the spec's
	//    capability list. Typos here are common and only surface at
	//    install time today.
	for i, cap := range m.Capabilities {
		if !spec.Capabilities[cap] {
			add(SeverityError, fmt.Sprintf("capabilities[%d]", i), fmt.Sprintf(
				"capability %q is not a recognized capability — check spelling against the public spec", cap))
		}
	}

	// 2. min_api_version sanity. A plugin asking for a version newer
	//    than the bundled spec means the validator can't fully judge
	//    drift — info-level note so the author knows to upgrade gen.
	if m.MinAPIVersion != "" && spec.APIVersion != "" {
		if SemverLess(spec.APIVersion, m.MinAPIVersion) {
			add(SeverityInfo, "min_api_version", fmt.Sprintf(
				"plugin requires API >= %s but this branchkit-gen knows API %s — drift checks will be incomplete; install a newer branchkit-gen",
				m.MinAPIVersion, spec.APIVersion))
		}
	}

	// 3. For each method the plugin actually calls, check it against
	//    the spec. Static analysis is best-effort: runtime-computed
	//    method names slip through silently.
	declaredMin := m.MinAPIVersion

	// Deduplicate so a method called from many sites only produces one issue.
	seen := map[string]CalledMethod{}
	var order []string
	for _, c := range called {
		if _, ok := seen[c.Name]; ok {
			continue
		}
		seen[c.Name] = c
		order = append(order, c.Name)
	}
	sort.Strings(order)

	for _, name := range order {
		c := seen[name]
		method, exists := spec.Methods[name]
		if !exists {
			// Unknown method: could be a typo, could be a plugin-to-plugin
			// call (which the open RPC spec doesn't enumerate). Info-level
			// so authors aren't spammed with false positives.
			add(SeverityInfo, locField(c), fmt.Sprintf(
				"method %q is not in the platform spec — typo, or a plugin-to-plugin call", name))
			continue
		}

		// Removal check (strongest signal)
		if method.RemovedIn != "" && SemverLessOrEqual(method.RemovedIn, declaredMin) {
			add(SeverityError, locField(c), fmt.Sprintf(
				"method %q was removed in API %s; plugin's min_api_version is %s — this call will fail at runtime",
				name, method.RemovedIn, declaredMin))
			continue
		}

		// Since check: method newer than the declared minimum.
		if method.Since != "" && SemverLess(declaredMin, method.Since) {
			add(SeverityError, locField(c), fmt.Sprintf(
				"method %q was introduced in API %s but plugin's min_api_version is %s — bump min_api_version to >= %s",
				name, method.Since, declaredMin, method.Since))
			continue
		}

		// Deprecation: still callable, but should migrate.
		if method.DeprecatedIn != "" && SemverLessOrEqual(method.DeprecatedIn, currentSpecLatest(spec)) {
			add(SeverityWarn, locField(c), fmt.Sprintf(
				"method %q is deprecated as of API %s — plan a migration before it is removed",
				name, method.DeprecatedIn))
		}
	}

	return issues
}

// locField formats a callsite as "file:line" so messages point at the
// exact line. Falls back to "calls.<method>" when no position info.
func locField(c CalledMethod) string {
	if c.File == "" {
		return "calls." + c.Name
	}
	return fmt.Sprintf("%s:%d", c.File, c.Line)
}

// currentSpecLatest returns the spec's API version. Named separately so
// the deprecation comparison is readable: "deprecated_in <= latest_known_version"
// means "the deprecation is in effect on this actuator."
func currentSpecLatest(s *Spec) string {
	return s.APIVersion
}

// PlatformMethodNames returns the sorted list of method names known to
// the spec. Useful for debugging tooling and not part of the validation
// output proper.
func PlatformMethodNames(s *Spec) []string {
	names := make([]string, 0, len(s.Methods))
	for n := range s.Methods {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}
