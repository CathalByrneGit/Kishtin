// Command licenses writes the licence notices built into kishtind and
// kishtin: Kishtin's own licence and NOTICE, then the licence of every Go
// module the programs link and every npm package the screens are built
// from. MIT and BSD licences ask for their notices to go with binary copies;
// the release archives hold only the two programs, so the notices travel
// inside them and `kishtind licenses` prints them.
//
// Run from the repository root, after `make frontend-deps`:
//
//	go run ./scripts/licenses > internal/legal/licenses.txt
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type pkg struct {
	name, version string
	files         []string // licence files, read in this order
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "licenses:", err)
		os.Exit(1)
	}
}

func run(w io.Writer) error {
	own, err := ownNotices()
	if err != nil {
		return err
	}
	gomods, err := goModules()
	if err != nil {
		return err
	}
	npms, err := npmPackages("frontend")
	if err != nil {
		return err
	}
	var b bytes.Buffer
	b.WriteString(own)
	section(&b, "Go modules linked into kishtind and kishtin", gomods)
	section(&b, "npm packages the screens are built from", npms)
	_, err = w.Write(b.Bytes())
	return err
}

func ownNotices() (string, error) {
	lic, err := os.ReadFile("LICENSE")
	if err != nil {
		return "", fmt.Errorf("run from the repository root: %w", err)
	}
	notice, err := os.ReadFile("NOTICE")
	if err != nil {
		return "", err
	}
	return string(notice) + "\n" + rule + "Kishtin is licensed under the Apache License, Version 2.0:\n\n" + string(lic), nil
}

const rule = "\n================================================================================\n"

func section(b *bytes.Buffer, title string, ps []pkg) {
	fmt.Fprintf(b, "%s%s (%d)%s", rule, title, len(ps), rule)
	for _, p := range ps {
		fmt.Fprintf(b, "\n--- %s %s ---\n", p.name, p.version)
		if len(p.files) == 0 {
			b.WriteString("\n(No licence file was found in this package. See its source for its terms.)\n")
			continue
		}
		for _, f := range p.files {
			text, err := os.ReadFile(f)
			if err != nil {
				fmt.Fprintf(b, "\n(Could not read %s: %v)\n", filepath.Base(f), err)
				continue
			}
			b.WriteString("\n")
			b.Write(bytes.TrimRight(text, "\n"))
			b.WriteString("\n")
		}
	}
}

// licenceFiles finds the licence and notice files at the top of dir.
func licenceFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := strings.ToUpper(e.Name())
		if e.IsDir() {
			continue
		}
		for _, prefix := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE", "UNLICENSE"} {
			if strings.HasPrefix(n, prefix) {
				out = append(out, filepath.Join(dir, e.Name()))
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

type goModule struct {
	Path, Version, Dir string
	Main               bool
}

func goModules() ([]pkg, error) {
	cmd := exec.Command("go", "list", "-deps", "-json=Module", "./cmd/...")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	seen := map[string]pkg{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var v struct{ Module *goModule }
		if err := dec.Decode(&v); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("read go list: %w", err)
		}
		m := v.Module
		if m == nil || m.Main {
			continue
		}
		if _, ok := seen[m.Path]; !ok {
			seen[m.Path] = pkg{name: m.Path, version: m.Version, files: licenceFiles(m.Dir)}
		}
	}
	return sorted(seen), nil
}

type packageJSON struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

// npmPackages walks the screens' runtime dependencies, and theirs, through
// node_modules. Build tools are left out: they are not part of the output.
func npmPackages(root string) ([]pkg, error) {
	var top packageJSON
	if err := readJSON(filepath.Join(root, "package.json"), &top); err != nil {
		return nil, err
	}
	seen := map[string]pkg{}
	type item struct{ name, from string }
	queue := []item{}
	for name := range top.Dependencies {
		queue = append(queue, item{name, root})
	}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		dir, ok := resolve(it.name, it.from, root)
		if !ok {
			return nil, fmt.Errorf("npm package %s is not installed: run make frontend-deps", it.name)
		}
		var p packageJSON
		if err := readJSON(filepath.Join(dir, "package.json"), &p); err != nil {
			return nil, err
		}
		key := p.Name + "@" + p.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = pkg{name: p.Name, version: p.Version, files: licenceFiles(dir)}
		for dep := range p.Dependencies {
			queue = append(queue, item{dep, dir})
		}
	}
	return sorted(seen), nil
}

// resolve finds a package the way Node does: in node_modules next to the
// package that needs it, then in each folder above, up to root.
func resolve(name, from, root string) (string, bool) {
	rootAbs, _ := filepath.Abs(root)
	dir, _ := filepath.Abs(from)
	for {
		cand := filepath.Join(dir, "node_modules", name)
		if _, err := os.Stat(filepath.Join(cand, "package.json")); err == nil {
			return cand, true
		}
		if dir == rootAbs || filepath.Dir(dir) == dir {
			break
		}
		dir = filepath.Dir(dir)
	}
	cand := filepath.Join(rootAbs, "node_modules", name)
	if _, err := os.Stat(filepath.Join(cand, "package.json")); err == nil {
		return cand, true
	}
	return "", false
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func sorted(m map[string]pkg) []pkg {
	out := make([]pkg, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].version < out[j].version
	})
	return out
}
