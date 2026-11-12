package devtools

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Module identifies an independently validated runtime or tooling module.
type Module struct{ Kind, Directory string }

// Modules reads the authoritative module inventory and rejects duplicate or unsafe paths.
func Modules(root string) ([]Module, error) {
	data, err := os.ReadFile(filepath.Join(root, "scripts", "module-manifest.txt"))
	if err != nil {
		return nil, err
	}
	var modules []Module
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[0] != "published" && fields[0] != "tooling") || filepath.IsAbs(fields[1]) || strings.Contains(fields[1], "..") || seen[fields[1]] {
			return nil, errors.New("invalid module inventory")
		}
		seen[fields[1]] = true
		modules = append(modules, Module{fields[0], fields[1]})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !seen["."] {
		return nil, errors.New("root module missing from inventory")
	}
	return modules, nil
}

// CheckModules prevents nested module additions from escaping tests and release planning.
func CheckModules(root string) error {
	modules, err := Modules(root)
	if err != nil {
		return err
	}
	var expected, found []string
	for _, module := range modules {
		expected = append(expected, filepath.ToSlash(filepath.Join(module.Directory, "go.mod")))
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == ".artifacts") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "go.mod" {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(expected)
	sort.Strings(found)
	if strings.Join(expected, "\n") != strings.Join(found, "\n") {
		return fmt.Errorf("module inventory mismatch: expected %v, found %v", expected, found)
	}
	return nil
}

// EachModule runs the same validation in every relevant module without crossing boundaries implicitly.
func EachModule(root, operation string) error {
	modules, err := Modules(root)
	if err != nil {
		return err
	}
	for _, module := range modules {
		if module.Directory == "integration" {
			continue
		}
		arguments := []string{operation, "./..."}
		switch operation {
		case "test-race":
			arguments = []string{"test", "-race", "./..."}
		case "tidy":
			arguments = []string{"mod", "tidy"}
		}
		output, err := goCommand(filepath.Join(root, module.Directory), append(os.Environ(), "GOWORK=off"), arguments...)
		fmt.Print(string(output))
		if err != nil {
			return err
		}
	}
	return nil
}
