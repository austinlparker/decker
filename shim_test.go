package decker

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// grandfatheredShims predate TestNoCompatShims. The list only shrinks:
// delete an entry together with its shim, and add one only with the
// maintainer's say-so in the PR that adds the shim.
var grandfatheredShims = []string{}

// shimPackages are the packages whose exported API talks import, by import
// path, with their directories.
var shimPackages = []struct{ path, dir string }{
	{"github.com/austinlparker/decker", "."},
	{"github.com/austinlparker/decker/decktest", "decktest"},
}

// TestNoCompatShims fails on exported API whose only job is to keep an old
// spelling compiling: a type alias, a declaration documented as Deprecated,
// a var or const that is another name for a function or constant, or a
// function or method that only passes its own arguments on to another
// exported one. Before 1.0 decker changes its API in place instead: move the
// callers (gallery, examples, docs) to the new form, delete the old one, and
// say in the PR that it breaks.
func TestNoCompatShims(t *testing.T) {
	fset := token.NewFileSet()
	dirs := make([]string, len(shimPackages))
	for i, p := range shimPackages {
		dirs[i] = "./" + p.dir
	}
	imp := exportImporter(t, fset, dirs...)
	found := map[string]string{}
	for _, p := range shimPackages {
		maps.Copy(found, findShims(t, fset, imp, p.path, p.dir))
	}
	allowed := map[string]bool{}
	for _, name := range grandfatheredShims {
		allowed[name] = true
		if _, ok := found[name]; !ok {
			t.Errorf("%s is no longer a shim: remove it from grandfatheredShims", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(found)) {
		if !allowed[name] {
			t.Errorf("%s looks like a compatibility shim: %s.\n"+
				"Before 1.0 decker changes its API in place: move every caller (gallery, examples, docs) "+
				"to the new form, delete the old one, and note the break in the PR. "+
				"See \"API changes\" in AGENTS.md.", name, found[name])
		}
	}
}

// exportImporter imports the dependencies of pkgs from the export data the
// go command already built for them, as go list -export reports it. Type
// checking them from source instead takes seconds.
func exportImporter(t *testing.T, fset *token.FileSet, pkgs ...string) types.Importer {
	t.Helper()
	args := append([]string{"list", "-export", "-deps", "-f", "{{.ImportPath}}\t{{.Export}}"}, pkgs...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		t.Fatalf("go list -export: %v", err)
	}
	exports := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if path, file, ok := strings.Cut(line, "\t"); ok && file != "" {
			exports[path] = file
		}
	}
	return importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
}

// findShims type-checks the non-test files of the package in dir and returns
// its shim-like exported identifiers, qualified by the package's name, with
// where and why.
func findShims(t *testing.T, fset *token.FileSet, imp types.Importer, path, dir string) map[string]string {
	t.Helper()
	name := path[strings.LastIndex(path, "/")+1:]
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		if n := e.Name(); e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if f.Name.Name == name {
			files = append(files, f)
		}
	}
	info := &types.Info{
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, err := (&types.Config{Importer: imp}).Check(path, fset, files, info)
	if err != nil {
		t.Fatalf("type-checking %s: %v", path, err)
	}

	out := map[string]string{}
	add := func(pos token.Pos, obj types.Object, why string) {
		if n, ok := apiName(obj); ok {
			out[name+"."+n] = fset.Position(pos).String() + ": " + why
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				fn := info.Defs[d.Name].(*types.Func)
				if deprecated(d.Doc) {
					add(d.Pos(), fn, "its doc says Deprecated")
				} else if to := forwardsTo(d, fn, info); to != nil {
					add(d.Pos(), fn, "it only passes its arguments on to "+to.Name())
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					doc := specDoc(d, s)
					switch s := s.(type) {
					case *ast.TypeSpec:
						tn := info.Defs[s.Name].(*types.TypeName)
						switch {
						case deprecated(doc):
							add(s.Pos(), tn, "its doc says Deprecated")
						case tn.IsAlias():
							add(s.Pos(), tn, "it is another name for "+types.TypeString(types.Unalias(tn.Type()), types.RelativeTo(pkg)))
						}
						if st, ok := s.Type.(*ast.StructType); ok && tn.Exported() {
							for _, fld := range st.Fields.List {
								for _, n := range fld.Names {
									if n.IsExported() && (deprecated(fld.Doc) || deprecated(fld.Comment)) {
										out[name+"."+tn.Name()+"."+n.Name] = fset.Position(n.Pos()).String() + ": its doc says Deprecated"
									}
								}
							}
						}
					case *ast.ValueSpec:
						for i, n := range s.Names {
							obj := info.Defs[n]
							switch {
							case deprecated(doc) || deprecated(s.Comment):
								add(n.Pos(), obj, "its doc says Deprecated")
							case i < len(s.Values):
								// A var copying another var's value is a
								// default, not a second name; one holding a
								// function, or a const naming a const, is.
								switch to := used(s.Values[i], info).(type) {
								case *types.Func:
									if _, isVar := obj.(*types.Var); isVar && sameAPI(to, pkg) {
										add(n.Pos(), obj, "it is another name for "+to.Name())
									}
								case *types.Const:
									if _, isConst := obj.(*types.Const); isConst && sameAPI(to, pkg) {
										add(n.Pos(), obj, "it is another name for "+to.Name())
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return out
}

// apiName is obj's name as a talk sees it, Type.Name for a method, and
// whether it is part of the API: exported, and on an exported type.
func apiName(obj types.Object) (string, bool) {
	if !obj.Exported() {
		return "", false
	}
	if fn, ok := obj.(*types.Func); ok {
		if recv := fn.Type().(*types.Signature).Recv(); recv != nil {
			return typeOf(recv.Type(), fn.Name())
		}
	}
	return obj.Name(), true
}

// typeOf names member on the named type t (or *t), and reports whether that
// type is exported.
func typeOf(t types.Type, member string) (string, bool) {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok || !n.Obj().Exported() {
		return "", false
	}
	return n.Obj().Name() + "." + member, true
}

// specDoc is a spec's doc comment, or its declaration's when the declaration
// has just the one spec.
func specDoc(d *ast.GenDecl, s ast.Spec) *ast.CommentGroup {
	var doc *ast.CommentGroup
	switch s := s.(type) {
	case *ast.TypeSpec:
		doc = s.Doc
	case *ast.ValueSpec:
		doc = s.Doc
	}
	if doc == nil && len(d.Specs) == 1 {
		doc = d.Doc
	}
	return doc
}

// used is the object an identifier or selector expression refers to.
func used(e ast.Expr, info *types.Info) types.Object {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		return info.Uses[e]
	case *ast.SelectorExpr:
		return info.Uses[e.Sel]
	}
	return nil
}

// sameAPI reports whether obj is exported API of pkg.
func sameAPI(obj types.Object, pkg *types.Package) bool {
	_, ok := apiName(obj)
	return ok && obj.Pkg() == pkg
}

func deprecated(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, line := range strings.Split(doc.Text(), "\n") {
		if strings.HasPrefix(line, "Deprecated:") {
			return true
		}
	}
	return false
}

// forwardsTo returns the exported function or method of the same package
// that fn's whole body calls with exactly fn's own parameters, in order (and
// for a method, on fn's own receiver): two names for one thing. It returns
// nil for anything else.
func forwardsTo(d *ast.FuncDecl, fn *types.Func, info *types.Info) *types.Func {
	if d.Body == nil || len(d.Body.List) != 1 {
		return nil
	}
	var call *ast.CallExpr
	switch s := d.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 1 {
			call, _ = ast.Unparen(s.Results[0]).(*ast.CallExpr)
		}
	case *ast.ExprStmt:
		call, _ = ast.Unparen(s.X).(*ast.CallExpr)
	}
	if call == nil {
		return nil
	}
	sig := fn.Type().(*types.Signature)
	var target *types.Func
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		target, _ = info.Uses[f].(*types.Func)
	case *ast.SelectorExpr:
		sel := info.Selections[f]
		if sel == nil { // a package-qualified function
			target, _ = info.Uses[f.Sel].(*types.Func)
			break
		}
		if sel.Kind() != types.MethodVal || sig.Recv() == nil || used(f.X, info) != sig.Recv() {
			return nil
		}
		target, _ = sel.Obj().(*types.Func)
	}
	if target == nil || target == fn || !sameAPI(target, fn.Pkg()) {
		return nil
	}
	params := sig.Params()
	if len(call.Args) != params.Len() || call.Ellipsis.IsValid() != sig.Variadic() {
		return nil
	}
	for i, a := range call.Args {
		if used(a, info) != params.At(i) {
			return nil
		}
	}
	return target
}

// TestShimDetector checks findShims on code it must flag and code it must
// leave alone, so a quiet TestNoCompatShims means something.
func TestShimDetector(t *testing.T) {
	dir := t.TempDir()
	src := `package fake

type Font struct{}
type FontOld = Font // alias
type font struct{}

// Old is gone.
//
// Deprecated: use New.
func Old() {}

func Load(name string) *Font { return nil }
func LoadFont(name string) *Font { return Load(name) }
func OpenFont(name string) *Font { return (Load)(name) }
func load(name string) *Font { return Load(name) }
func Open(name string) *Font { return load(name) }
func Join(parts ...string) string { return Concat(parts...) }
func Concat(parts ...string) string { return "" }
func Swap(a, b string) string { return Pair(b, a) }
func Pair(a, b string) string { return a + b }
func Call(f func(string) *Font, name string) *Font { return f(name) }
var Parse = Load
var parse = Load
const Max = 3
const Limit = Max
var Default = Fallback
var Fallback = 2

func (f Font) Width(s string) int { return f.Measure(s) }
func (f Font) Measure(s string) int { return len(s) }
func (f Font) Height(s string) int { return f.Measure(s + "x") }
func (f Font) Other(g Font, s string) int { return g.Measure(s) }
func (f font) Width(s string) int { return f.Measure(s) }
func (f font) Measure(s string) int { return len(s) }

type Opts struct {
	// Size is ignored.
	//
	// Deprecated: set Scale.
	Size int
	Scale int
	// Deprecated: internal.
	old int
}
`
	if err := os.WriteFile(filepath.Join(dir, "fake.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(maps.Keys(findShims(t, token.NewFileSet(), nil, "example.com/fake", dir)))
	want := []string{
		"fake.Font.Width", "fake.FontOld", "fake.Join", "fake.Limit",
		"fake.LoadFont", "fake.Old", "fake.OpenFont", "fake.Opts.Size", "fake.Parse",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findShims flagged\n  %v\nwant\n  %v", got, want)
	}
}
