package main

import (
	"fmt"
	"strings"
)

// RenderTS produces the contents of actions_gen.ts from a plugin manifest.
func RenderTS(manifest *PluginManifest) string {
	names := sortedActionNames(manifest.ActionTypes)

	var b strings.Builder
	b.WriteString(actionsHeader("//"))
	b.WriteString("\n")
	// The registrars below name the SDK's Plugin as a type only, so the
	// import is erased at compile time and costs nothing at run time.
	if len(names) > 0 {
		b.WriteString("import type { Plugin } from \"@branchkitdev/plugin-sdk-ts\";\n\n")
	}

	prefix := ""
	if manifest.ActionPrefix != nil {
		prefix = *manifest.ActionPrefix
	}

	for _, name := range names {
		schema := manifest.ActionTypes[name]
		ifaceName := GoIdentifier(name)
		fullAction := name
		if prefix != "" {
			fullAction = prefix + "." + name
		}

		for _, field := range schema.Fields {
			if field.EffectiveFieldType() == FieldTypeEnum {
				enumName := enumTypeName(ifaceName, field.Key)
				renderTSEnum(&b, enumName, field.EnumValues)
				b.WriteString("\n")
			}
		}

		renderTSInterface(&b, ifaceName, fullAction, &schema)
		b.WriteString("\n")
		renderTSActionRegistrar(&b, ifaceName, fullAction, &schema)
		b.WriteString("\n")
	}

	return b.String()
}

func renderTSEnum(b *strings.Builder, typeName string, values []string) {
	if len(values) == 0 {
		return
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%q", v)
	}
	fmt.Fprintf(b, "export type %s = %s;\n", typeName, strings.Join(parts, " | "))
}

func renderTSInterface(b *strings.Builder, ifaceName, fullAction string, schema *ActionTypeSchema) {
	label := fullAction
	if schema.Label != "" {
		label = fmt.Sprintf("%s (%s)", fullAction, schema.Label)
	}
	fmt.Fprintf(b, "/** Params shape for action %q. */\n", label)
	fmt.Fprintf(b, "export interface %sParams {\n", ifaceName)
	for _, field := range schema.Fields {
		tsTy := tsFieldType(ifaceName, &field)
		optional := ""
		if !field.Required {
			optional = "?"
		}
		fmt.Fprintf(b, "  %s%s: %s;\n", field.Key, optional, tsTy)
	}
	b.WriteString("}\n")
}

func tsFieldType(ifaceName string, field *ActionFieldSchema) string {
	ft := field.EffectiveFieldType()
	switch ft {
	case FieldTypeString, FieldTypeSecretRef:
		return "string"
	case FieldTypeInt, FieldTypeNumber:
		return "number"
	case FieldTypeBoolean:
		return "boolean"
	case FieldTypeStringArray:
		return "string[]"
	case FieldTypeEnum:
		return enumTypeName(ifaceName, field.Key)
	case FieldTypeObject, FieldTypeJson:
		return "unknown"
	default:
		return "unknown"
	}
}

// renderTSActionRegistrar emits a typed registrar for one action, the
// TypeScript counterpart of the Go `Handle<Action>` and Python
// `handle_<action>` helpers: the action string and the params type both come
// from the manifest, so neither can drift from it at the call site.
//
//	handleGreet(plugin, async (req) => { req.params.name; });
//
// The handler's type is read off the SDK's own `handleAction`, instantiated
// with the params interface, so it is exactly what `handleAction<T>` takes.
// An action with no declared fields takes the untyped form, as in Go.
func renderTSActionRegistrar(b *strings.Builder, ifaceName, fullAction string, schema *ActionTypeSchema) {
	label := fullAction
	if schema.Label != "" {
		label = fmt.Sprintf("%s (%s)", fullAction, schema.Label)
	}
	fmt.Fprintf(b, "/** Register a typed handler for action %q. */\n", label)
	if len(schema.Fields) == 0 {
		fmt.Fprintf(b, "export function handle%s(\n  plugin: Plugin,\n  fn: Parameters<typeof plugin.handleAction>[1],\n): void {\n", ifaceName)
		fmt.Fprintf(b, "  plugin.handleAction(%q, fn);\n}\n", fullAction)
		return
	}
	fmt.Fprintf(b, "export function handle%s(\n  plugin: Plugin,\n  fn: Parameters<typeof plugin.handleAction<%sParams>>[1],\n): void {\n", ifaceName, ifaceName)
	fmt.Fprintf(b, "  plugin.handleAction<%sParams>(%q, fn);\n}\n", ifaceName, fullAction)
}
