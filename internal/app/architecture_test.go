package app_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModulesDoNotImportOtherInternals(t *testing.T) {
	root := moduleRoot(t)
	var failures []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return err
		}
		if info.IsDir() && (info.Name() == "vendor" || info.Name() == ".git" || info.Name() == "frontend" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		owner := moduleOf(path)
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			target, internal := internalModule(p)
			if !internal || target == owner {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			failures = append(failures, rel+" imports "+p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) > 0 {
		t.Fatalf("cross-module internal imports:\n%s", strings.Join(failures, "\n"))
	}
}

func moduleOf(path string) string {
	const prefix = "github.com/cng1985/edugo/"
	norm := filepath.ToSlash(path)
	idx := strings.Index(norm, "internal/modules/")
	if idx < 0 {
		return ""
	}
	parts := strings.Split(norm[idx+len("internal/modules/"):], "/")
	if parts[0] == "teaching" || parts[0] == "learning" {
		return prefix + "internal/modules/" + parts[0] + "/" + parts[1]
	}
	return prefix + "internal/modules/" + parts[0]
}

func internalModule(importPath string) (string, bool) {
	const marker = "internal/modules/"
	idx := strings.Index(importPath, marker)
	if idx < 0 {
		return "", false
	}
	parts := strings.Split(importPath[idx+len(marker):], "/")
	prefix := "github.com/cng1985/edugo/internal/modules/"
	if parts[0] == "teaching" || parts[0] == "learning" {
		if len(parts) >= 3 && parts[2] == "internal" {
			return prefix + parts[0] + "/" + parts[1], true
		}
		return "", false
	}
	if len(parts) >= 2 && parts[1] == "internal" {
		return prefix + parts[0], true
	}
	return "", false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
