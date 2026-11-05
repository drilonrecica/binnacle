// SPDX-License-Identifier: AGPL-3.0-only

// Package lint holds Binnacle's own static checks, built on the standard
// library so they add no dependency (RUN-7).
package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// Finding is one error value assigned to the blank identifier.
type Finding struct {
	Pos  token.Position
	Call string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s: error from %s discarded with _; handle, return or log it", f.Pos, f.Call)
}

var errorType = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// DiscardedErrors reports every assignment in files that gives a value
// implementing error to the blank identifier: `_ = f()`, `v, _ := g()` and
// `var _ = f()`. Type information decides, so `v, _ := m[k]` and
// `_, ok := x.(T)` are not reported.
func DiscardedErrors(fset *token.FileSet, files []*ast.File, info *types.Info) []Finding {
	var findings []Finding
	report := func(lhs []ast.Expr, rhs []ast.Expr) {
		for i, target := range lhs {
			ident, ok := target.(*ast.Ident)
			if !ok || ident.Name != "_" {
				continue
			}
			var value ast.Expr
			var typ types.Type
			switch {
			case len(lhs) == len(rhs):
				value = rhs[i]
				typ = info.TypeOf(value)
			case len(rhs) == 1:
				value = rhs[0]
				if tuple, ok := info.TypeOf(value).(*types.Tuple); ok && i < tuple.Len() {
					typ = tuple.At(i).Type()
				}
			}
			if typ == nil || !types.Implements(typ, errorType) || infallible(info, value) {
				continue
			}
			findings = append(findings, Finding{Pos: fset.Position(ident.Pos()), Call: describe(value)})
		}
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.AssignStmt:
				report(n.Lhs, n.Rhs)
			case *ast.ValueSpec:
				lhs := make([]ast.Expr, len(n.Names))
				for i, name := range n.Names {
					lhs[i] = name
				}
				report(lhs, n.Values)
			}
			return true
		})
	}
	return findings
}

// infallibleFuncs are documented never to return a non-nil error, so a
// caller has nothing to handle: crypto/rand.Read since Go 1.24, and the
// writers of hash.Hash, bytes.Buffer and strings.Builder.
var infallibleFuncs = map[string]bool{
	"crypto/rand.Read":               true,
	"(*bytes.Buffer).Write":          true,
	"(*bytes.Buffer).WriteString":    true,
	"(*bytes.Buffer).WriteByte":      true,
	"(*bytes.Buffer).WriteRune":      true,
	"(*strings.Builder).Write":       true,
	"(*strings.Builder).WriteString": true,
	"(*strings.Builder).WriteByte":   true,
	"(*strings.Builder).WriteRune":   true,
}

// infallible reports whether expr calls a function of infallibleFuncs.
func infallible(info *types.Info, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	var obj types.Object
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		obj = info.Uses[fun]
	case *ast.SelectorExpr:
		if selection, ok := info.Selections[fun]; ok {
			obj = selection.Obj()
			// A Write promoted from hash.Hash through an interface such as
			// the hash.Hash returned by hmac.New.
			if named, ok := types.Unalias(selection.Recv()).(*types.Named); ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "hash" && fun.Sel.Name == "Write" {
				return true
			}
		} else {
			obj = info.Uses[fun.Sel]
		}
	}
	fn, ok := obj.(*types.Func)
	return ok && infallibleFuncs[fn.FullName()]
}

// describe names the call that produced the discarded value.
func describe(expr ast.Expr) string {
	if call, ok := expr.(*ast.CallExpr); ok {
		expr = call.Fun
	}
	var buf bytes.Buffer
	if err := writeExpr(&buf, expr); err != nil {
		return "expression"
	}
	return buf.String()
}

func writeExpr(w io.Writer, expr ast.Expr) error {
	switch e := expr.(type) {
	case *ast.Ident:
		_, err := io.WriteString(w, e.Name)
		return err
	case *ast.SelectorExpr:
		if err := writeExpr(w, e.X); err != nil {
			return err
		}
		_, err := io.WriteString(w, "."+e.Sel.Name)
		return err
	case *ast.CallExpr:
		if err := writeExpr(w, e.Fun); err != nil {
			return err
		}
		_, err := io.WriteString(w, "()")
		return err
	}
	_, err := io.WriteString(w, "expression")
	return err
}

// listedPackage is the part of `go list -json` the loader needs.
type listedPackage struct {
	ImportPath string
	Dir        string
	Export     string
	GoFiles    []string
	CgoFiles   []string
	DepOnly    bool
	Error      *struct{ Err string }
}

// Check type-checks the non-test files of the packages matching patterns,
// run from dir, and returns their discarded errors sorted by position.
// Imports are read from the compiler's export data (`go list -export`), so
// the packages are type-checked exactly as the build sees them.
func Check(dir string, patterns ...string) ([]Finding, error) {
	args := append([]string{"list", "-e", "-export", "-deps", "-json=ImportPath,Dir,Export,GoFiles,CgoFiles,DepOnly,Error"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, stderr.String())
	}
	exports := map[string]string{}
	var targets []listedPackage
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode go list: %w", err)
		}
		if pkg.Error != nil {
			return nil, fmt.Errorf("package %s: %s", pkg.ImportPath, pkg.Error.Err)
		}
		exports[pkg.ImportPath] = pkg.Export
		if !pkg.DepOnly {
			targets = append(targets, pkg)
		}
	}
	fset := token.NewFileSet()
	imports := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok || file == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
	var findings []Finding
	for _, pkg := range targets {
		if len(pkg.CgoFiles) > 0 {
			return nil, fmt.Errorf("package %s uses cgo, which the checker does not type-check", pkg.ImportPath)
		}
		var files []*ast.File
		for _, name := range pkg.GoFiles {
			file, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
		config := types.Config{Importer: imports}
		if _, err := config.Check(pkg.ImportPath, fset, files, info); err != nil {
			return nil, fmt.Errorf("type-check %s: %w", pkg.ImportPath, err)
		}
		findings = append(findings, DiscardedErrors(fset, files, info)...)
	}
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i].Pos, findings[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.Line < b.Line
	})
	return findings, nil
}

// ErrFindings is returned by Run when the check found discarded errors.
var ErrFindings = errors.New("discarded errors found")

// Run prints the findings of Check to w and returns ErrFindings when there
// are any.
func Run(w io.Writer, dir string, patterns ...string) error {
	findings, err := Check(dir, patterns...)
	if err != nil {
		return err
	}
	for _, finding := range findings {
		if _, err := fmt.Fprintln(w, finding); err != nil {
			return err
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("%w: %d", ErrFindings, len(findings))
	}
	return nil
}
