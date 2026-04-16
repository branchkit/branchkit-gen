// branchkit-gen generates typed action param structs from BranchKit
// plugin.json manifests. The generated files (actions_gen.go for Go,
// actions_gen.ts for TypeScript) contain struct/interface definitions
// and typed enum constants derived from the manifest's action_types
// block — the single source of truth for each plugin's action surface.
//
// Usage:
//
//	branchkit-gen --plugin <dir>     Generate for one plugin
//	branchkit-gen --all              Generate for every plugin in plugins/
//
// Install:
//
//	go install github.com/branchkit/branchkit-gen@latest
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	pluginDir := flag.String("plugin", "", "Plugin directory containing plugin.json")
	all := flag.Bool("all", false, "Iterate subdirectories of current directory for plugin.json")
	flag.Parse()

	if *all {
		dirs := enumeratePluginDirs(".")
		run(dirs)
	} else if *pluginDir != "" {
		run([]string{*pluginDir})
	} else {
		fmt.Fprintln(os.Stderr, "usage: branchkit-gen --plugin <dir> | --all")
		os.Exit(2)
	}
}

func run(dirs []string) {
	totalGo, totalTS, skipped := 0, 0, 0

	for _, dir := range dirs {
		manifest, err := LoadManifest(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[branchkit-gen] %s: ERROR %v\n", dir, err)
			os.Exit(1)
		}

		if len(manifest.ActionTypes) == 0 {
			skipped++
			continue
		}

		srcDir := filepath.Join(dir, "src")

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
	}

	fmt.Fprintf(os.Stderr, "[branchkit-gen] summary: %d plugins, %d go, %d ts, %d skipped\n",
		len(dirs), totalGo, totalTS, skipped)
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
