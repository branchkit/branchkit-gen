package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writeTree creates files (path -> contents) under a fresh temp dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const layoutManifest = `{
  "id": "snippets",
  "min_api_version": "0.2.0",
  "run": %RUN%,
  "action_prefix": "snippets",
  "action_types": {
    "type": {
      "label": "Type snippet",
      "fields": [
        { "key": "text", "label": "Text", "field_type": "string" },
        { "key": "name", "label": "Snippet name", "field_type": "string" }
      ]
    }
  }
}`

func manifestWithRun(run string) string {
	return strings.Replace(layoutManifest, "%RUN%", `"`+run+`"`, 1)
}

// The three layouts `branchkit-cli dev init` scaffolds, and where each one's
// generated files must land. A scaffold the generator does not recognise is a
// tutorial that fails at its first regenerate step.
func TestRunEmitsForEveryScaffoldLayout(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
		never []string
	}{
		{
			name: "go scaffold",
			files: map[string]string{
				"plugin.json": manifestWithRun("./snippets-plugin"),
				"src/go.mod":  "module example.com/snippets\n",
				"src/main.go": "package main\n",
			},
			want:  []string{"src/actions_gen.go"},
			never: []string{"src/actions_gen.ts", "actions_gen.py", "src/actions_gen.py"},
		},
		{
			name: "typescript scaffold (package.json at the root, entry in src/)",
			files: map[string]string{
				"plugin.json":  manifestWithRun("./snippets-plugin"),
				"package.json": "{}",
				"src/index.ts": "export {};\n",
			},
			want:  []string{"src/actions_gen.ts"},
			never: []string{"actions_gen.ts", "src/actions_gen.go", "actions_gen.py"},
		},
		{
			name: "python scaffold (main.py at the root)",
			files: map[string]string{
				"plugin.json": manifestWithRun("python3 main.py"),
				"main.py":     "",
			},
			want:  []string{"actions_gen.py"},
			never: []string{"src/actions_gen.py", "src/actions_gen.ts", "src/actions_gen.go"},
		},
		{
			name: "older typescript layout (package.json in src/)",
			files: map[string]string{
				"plugin.json":      manifestWithRun("./snippets-plugin"),
				"src/package.json": "{}",
			},
			want: []string{"src/actions_gen.ts"},
		},
		{
			name: "older python layout (pyproject.toml in src/)",
			files: map[string]string{
				"plugin.json":        manifestWithRun("./snippets-plugin"),
				"src/pyproject.toml": "",
			},
			want: []string{"src/actions_gen.py"},
		},
		{
			name: "a root package.json with no TypeScript entry is tooling, not a plugin language",
			files: map[string]string{
				"plugin.json":  manifestWithRun("./snippets-plugin"),
				"package.json": "{}",
				"src/go.mod":   "module example.com/snippets\n",
			},
			want:  []string{"src/actions_gen.go"},
			never: []string{"actions_gen.ts", "src/actions_gen.ts"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeTree(t, tc.files)
			run([]string{dir})
			for _, rel := range tc.want {
				if !fileExists(filepath.Join(dir, rel)) {
					t.Errorf("expected %s to be generated", rel)
				}
			}
			for _, rel := range tc.never {
				if fileExists(filepath.Join(dir, rel)) {
					t.Errorf("did not expect %s", rel)
				}
			}
		})
	}
}

// Every generated file names the tool and the command that regenerates it,
// and nothing else: the same header lands in third-party plugins, which have
// no repository recipe to run.
func TestGeneratedHeadersNameTheTool(t *testing.T) {
	m := &PluginManifest{}
	prefix := "snippets"
	m.ActionPrefix = &prefix
	m.ActionTypes = map[string]ActionTypeSchema{
		"type": {Label: "Type snippet", Fields: []ActionFieldSchema{{Key: "text", FieldType: FieldTypeString}}},
	}
	outputs := map[string]string{
		"go": RenderGo(m),
		"ts": RenderTS(m),
		"py": RenderPy(m),
	}
	goGenerated := regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)
	if !goGenerated.MatchString(outputs["go"]) {
		t.Errorf("Go output lacks the standard generated-code line:\n%s", outputs["go"])
	}
	for lang, out := range outputs {
		for _, stale := range []string{"emit-sdk", "just gen-plugins"} {
			if strings.Contains(out, stale) {
				t.Errorf("%s output mentions %q:\n%s", lang, stale, out)
			}
		}
		if !strings.Contains(out, "branchkit-gen --plugin .") {
			t.Errorf("%s output does not name the regenerate command:\n%s", lang, out)
		}
	}
	for _, h := range []string{collectionsHeader("//"), collectionsHeader("#")} {
		if strings.Contains(h, "just gen-plugins") || !strings.Contains(h, "branchkit-gen --plugin") {
			t.Errorf("collections header does not name the regenerate command:\n%s", h)
		}
	}
}
