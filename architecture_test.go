package devlb_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureLayerConstraints(t *testing.T) {
	allowed := map[string][]string{
		"model":    {},
		"label":    {},
		"config":   {},
		"proxy":    {},
		"portswap": {},
		"exec":     {"daemon", "label", "portswap"},
		"daemon":   {"config", "proxy"},
		"tui":      {"daemon"},
	}

	const modulePath = "github.com/takaaki-s/devlb/internal/"

	for pkg, allowedDeps := range allowed {
		t.Run(pkg, func(t *testing.T) {
			dir := filepath.Join("internal", pkg)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read dir %s: %v", dir, err)
			}

			allowedSet := make(map[string]bool, len(allowedDeps))
			for _, d := range allowedDeps {
				allowedSet[d] = true
			}

			fset := token.NewFileSet()
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				filePath := filepath.Join(dir, entry.Name())
				f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
				if err != nil {
					t.Fatalf("parse %s: %v", filePath, err)
				}

				for _, imp := range f.Imports {
					path := strings.Trim(imp.Path.Value, `"`)
					if !strings.HasPrefix(path, modulePath) {
						continue
					}
					dep := strings.TrimPrefix(path, modulePath)
					if i := strings.Index(dep, "/"); i >= 0 {
						dep = dep[:i]
					}
					if !allowedSet[dep] {
						t.Errorf("%s imports internal/%s, which is not allowed (allowed: %v)",
							filePath, dep, allowedDeps)
					}
				}
			}
		})
	}
}
