// Package apilist inventories source-level exported identifiers without loading
// dependencies or initializing graphics. It is development tooling, not a library API.
package apilist

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Snapshot lists every importable package's declared exports across all Go files,
// including OS and build-tag variants. Nested modules and implementation-only
// directories are omitted. It does not calculate promoted methods or type identity.
func Snapshot(fsys fs.FS, module string) (string, error) {
	entries := make(map[string]struct{})
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if name != "." {
				switch d.Name() {
				case "internal", "cmd", "testdata", "vendor", "examples", "api":
					return fs.SkipDir
				}
				if strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") {
					return fs.SkipDir
				}
				if _, err := fs.Stat(fsys, path.Join(name, "go.mod")); err == nil {
					return fs.SkipDir
				} else if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		source, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
		if err != nil {
			return err
		}
		if file.Name.Name == "main" {
			return nil
		}
		pkg := module
		if dir := path.Dir(name); dir != "." {
			pkg += "/" + dir
		}
		add := func(kind, identifier string) {
			entries[pkg+" "+kind+" "+identifier] = struct{}{}
		}
		add("package", file.Name.Name)
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() {
					continue
				}
				if decl.Recv == nil {
					add("func", decl.Name.Name)
				} else if receiver := typeName(decl.Recv.List[0].Type); receiver != "" {
					add("method", receiver+"."+decl.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if name.IsExported() {
								add(decl.Tok.String(), name.Name)
							}
						}
					case *ast.TypeSpec:
						if spec.Name.IsExported() {
							add("type", spec.Name.Name)
							if spec.Assign.IsValid() {
								if err := aliasMembers(fsys, module, path.Dir(name), file, spec.Type, spec.Name.Name, add, map[string]bool{}); err != nil {
									return err
								}
							}
						}
						switch typ := spec.Type.(type) {
						case *ast.StructType:
							addFields(add, "field", spec.Name.Name, typ.Fields)
						case *ast.InterfaceType:
							addFields(add, "interface", spec.Name.Name, typ.Methods)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inventory public identifiers: %w", err)
	}
	lines := make([]string, 0, len(entries))
	for entry := range entries {
		lines = append(lines, entry)
	}
	slices.Sort(lines)
	return "# Declared public identifiers; all OS/build-tag variants; see docs/development/api.md.\n" + strings.Join(lines, "\n") + "\n", nil
}

// aliasMembers includes the contract exposed through local aliases, even when
// its declaration lives in internal. It deliberately does not load dependencies
// or calculate promoted methods. Only referenced files are parsed.
func aliasMembers(fsys fs.FS, module, dir string, file *ast.File, typ ast.Expr, owner string, add func(string, string), seen map[string]bool) error {
	name := typeName(typ)
	if selector, ok := typ.(*ast.SelectorExpr); ok {
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return nil
		}
		found := false
		for _, im := range file.Imports {
			importPath, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				return err
			}
			local := path.Base(importPath)
			if im.Name != nil {
				local = im.Name.Name
			}
			if local != qualifier.Name {
				continue
			}
			if !strings.HasPrefix(importPath, module+"/") {
				return nil
			}
			dir, found = strings.TrimPrefix(importPath, module+"/"), true
			break
		}
		if !found {
			return nil
		}
	}
	key := dir + "." + name
	if seen[key] {
		return nil
	}
	seen[key] = true
	files, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filename := path.Join(dir, entry.Name())
		source, err := fs.ReadFile(fsys, filename)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filename, source, 0)
		if err != nil {
			return err
		}
		for _, decl := range parsed.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv != nil && decl.Name.IsExported() && typeName(decl.Recv.List[0].Type) == name {
					add("method", owner+"."+decl.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					target, ok := spec.(*ast.TypeSpec)
					if !ok || target.Name.Name != name {
						continue
					}
					if target.Assign.IsValid() {
						if err := aliasMembers(fsys, module, dir, parsed, target.Type, owner, add, seen); err != nil {
							return err
						}
					}
					switch t := target.Type.(type) {
					case *ast.StructType:
						addFields(add, "field", owner, t.Fields)
					case *ast.InterfaceType:
						addFields(add, "interface", owner, t.Methods)
					}
				}
			}
		}
	}
	return nil
}

func addFields(add func(string, string), kind, owner string, fields *ast.FieldList) {
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			if name := typeName(field.Type); ast.IsExported(name) {
				add(kind+"-embed", owner+"."+name)
			}
		}
		for _, name := range field.Names {
			if name.IsExported() {
				add(kind, owner+"."+name.Name)
			}
		}
	}
}

func typeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return typeName(expr.X)
	case *ast.IndexExpr:
		return typeName(expr.X)
	case *ast.IndexListExpr:
		return typeName(expr.X)
	case *ast.SelectorExpr:
		return expr.Sel.Name
	default:
		return ""
	}
}
