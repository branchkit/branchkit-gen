// branchkit-gen generates typed action param structs and validates
// BranchKit plugin manifests.
//
// Usage:
//
//	branchkit-gen --plugin <dir>     Generate for one plugin
//	branchkit-gen --all              Generate for every plugin in cwd
//	branchkit-gen validate [dir]     Validate plugin.json (defaults to cwd)
//	branchkit-gen validate --all     Validate every plugin under cwd
//
// Install:
//
//	go install github.com/branchkit/branchkit-gen@latest
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "validate" {
		os.Exit(runValidate(os.Args[2:]))
	}

	pluginDir := flag.String("plugin", "", "Plugin directory containing plugin.json")
	all := flag.Bool("all", false, "Iterate subdirectories of current directory for plugin.json")
	flag.Parse()

	if *all {
		dirs := enumeratePluginDirs(".")
		run(dirs)
	} else if *pluginDir != "" {
		run([]string{*pluginDir})
	} else {
		fmt.Fprintln(os.Stderr, "usage: branchkit-gen --plugin <dir> | --all | validate [dir]")
		os.Exit(2)
	}
}

// runValidate handles `branchkit-gen validate ...`. Returns the
// process exit code: 0 if no errors, 1 if any error issues, 2 on
// usage problems or unreadable manifests.
func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	all := fs.Bool("all", false, "Validate every subdirectory containing a plugin.json")
	asJSON := fs.Bool("json", false, "Emit issues as a JSON array on stdout")
	noDrift := fs.Bool("no-drift", false, "Skip static-analysis drift checks against the embedded spec")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var dirs []string
	if *all {
		dirs = enumeratePluginDirs(".")
	} else if fs.NArg() == 0 {
		dirs = []string{"."}
	} else {
		dirs = fs.Args()
	}

	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "[branchkit-gen] no plugin directories found")
		return 2
	}

	var spec *Spec
	if !*noDrift {
		s, err := LoadEmbeddedSpec()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[branchkit-gen] embedded spec is malformed: %v\n", err)
			return 2
		}
		spec = s
	}

	var allIssues []Issue
	exitCode := 0
	for _, dir := range dirs {
		manifest, raw, err := LoadManifestRaw(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[branchkit-gen] %s: %v\n", dir, err)
			exitCode = 2
			continue
		}
		issues := Validate(manifest, raw)

		if spec != nil {
			called, _ := AnalyzeGoSource(dir)
			issues = append(issues, CheckDrift(manifest, called, spec)...)
		}

		allIssues = append(allIssues, issues...)
		if HasErrors(issues) {
			exitCode = 1
		}
	}

	if *asJSON {
		out, err := json.MarshalIndent(allIssues, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[branchkit-gen] json encode: %v\n", err)
			return 2
		}
		fmt.Println(string(out))
		return exitCode
	}

	fmt.Printf("Validated %d plugin(s)\n\n", len(dirs))
	fmt.Print(formatHuman(allIssues))
	return exitCode
}

func run(dirs []string) {
	totalGo, totalTS, totalPy, skipped := 0, 0, 0, 0
	totalColl := 0

	// Built once for the whole run: every consumer's field claim is typed
	// against the same provider index, so two plugins reading one collection
	// cannot generate disagreeing structs for it.
	providers := resolveProviders(".")

	for _, dir := range dirs {
		manifest, err := LoadManifest(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[branchkit-gen] %s: ERROR %v\n", dir, err)
			os.Exit(1)
		}

		srcDir := filepath.Join(dir, "src")

		// Collections are emitted independently of action types: a plugin
		// that reads another's data but declares no actions of its own is a
		// perfectly ordinary plugin, and skipping it here would have made the
		// generated shapes quietly incomplete.
		if fileExists(filepath.Join(srcDir, "go.mod")) {
			if contents := RenderGoCollections(manifest, providers); contents != "" {
				path := filepath.Join(srcDir, "collections_gen.go")
				if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "[branchkit-gen] write %s: %v\n", path, err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "[branchkit-gen] wrote %s\n", path)
				totalColl++
			}
		}

		if fileExists(filepath.Join(srcDir, "package.json")) {
			if contents := RenderTSCollections(manifest, providers); contents != "" {
				path := filepath.Join(srcDir, "collections_gen.ts")
				if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "[branchkit-gen] write %s: %v\n", path, err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "[branchkit-gen] wrote %s\n", path)
				totalColl++
			}
		}
		if fileExists(filepath.Join(srcDir, "pyproject.toml")) {
			if contents := RenderPyCollections(manifest, providers); contents != "" {
				path := filepath.Join(srcDir, "collections_gen.py")
				if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "[branchkit-gen] write %s: %v\n", path, err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "[branchkit-gen] wrote %s\n", path)
				totalColl++
			}
		}

		if len(manifest.ActionTypes) == 0 {
			skipped++
			continue
		}

		// Emit Go if src/go.mod exists.
		if fileExists(filepath.Join(srcDir, "go.mod")) {
			contents := RenderGo(manifest)
			path := filepath.Join(srcDir, "actions_gen.go")
			if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "[branchkit-gen] write %s: %v\n", path, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[branchkit-gen] wrote %s\n", path)
			totalGo++
		}

		// Emit TS if src/package.json exists.
		if fileExists(filepath.Join(srcDir, "package.json")) {
			contents := RenderTS(manifest)
			path := filepath.Join(srcDir, "actions_gen.ts")
			if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "[branchkit-gen] write %s: %v\n", path, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[branchkit-gen] wrote %s\n", path)
			totalTS++
		}

		// Emit Python if the plugin runs under Python (manifest `run`
		// starts with python) or ships a main.py. Lands next to the
		// entrypoint: src/ when the source lives there, plugin root
		// otherwise.
		if strings.HasPrefix(manifest.Run, "python") ||
			fileExists(filepath.Join(dir, "main.py")) ||
			fileExists(filepath.Join(srcDir, "main.py")) {
			out := dir
			if fileExists(filepath.Join(srcDir, "main.py")) {
				out = srcDir
			}
			contents := RenderPy(manifest)
			path := filepath.Join(out, "actions_gen.py")
			if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "[branchkit-gen] write %s: %v\n", path, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[branchkit-gen] wrote %s\n", path)
			totalPy++
		}
	}

	fmt.Fprintf(os.Stderr,
		"[branchkit-gen] summary: %d plugins, %d go, %d ts, %d py, %d collections, %d skipped\n",
		len(dirs), totalGo, totalTS, totalPy, totalColl, skipped)
}

// enumeratePluginDirs lists subdirectories of root that contain a plugin.json.
func enumeratePluginDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[branchkit-gen] cannot read %s: %v\n", root, err)
		os.Exit(1)
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if fileExists(filepath.Join(dir, "plugin.json")) {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
