package decker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// grandfatheredShims predate TestNoCompatShims. The list only shrinks:
// delete an entry together with its shim, and add one only with the
// maintainer's say-so in the PR that adds the shim.
var grandfatheredShims = []string{
	"decker.FigFont",
	"decker.LoadFigFont",
	"decker.ParseFigFont",
	"decker.StockFigFont",
	"decker.StockFigFontNames",
}

// shimPackages are the directories whose exported API talks import.
var shimPackages = map[string]string{".": "decker", "decktest": "decktest"}

// variantSuffix marks a name that sits beside an older one instead of
// replacing it: DrawV2, FitEx, ParseLegacy.
var variantSuffix = regexp.MustCompile(`^(.+?)(V[0-9]+|Ex|Compat|Legacy|Old|New|WithOptions)$`)

// TestNoCompatShims fails on exported API whose only job is to keep an old
// spelling compiling: an alias, a deprecated declaration, a function or
// method that forwards its arguments to another exported one, or a variant
// like FooV2 beside Foo. Before 1.0 decker changes its API in place instead:
// move the callers (gallery, examples, docs) to the new form, delete the old
// one, and say in the PR that it breaks.
func TestNoCompatShims(t *testing.T) {
	found := map[string]string{}
	for dir, pkg := range shimPackages {
		maps.Copy(found, findShims(t, dir, pkg))
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

// findShims parses the non-test files of the package in dir and returns its
// shim-like exported identifiers, qualified by pkg, with where and why.
func findShims(t *testing.T, dir, pkg string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
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
		if f.Name.Name == pkg {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		t.Fatalf("no package %s in %s", pkg, dir)
	}

	// What every exported name is, so a forward or alias can be told from a
	// call into the implementation.
	funcs, consts, names := map[string]bool{}, map[string]bool{}, map[string]token.Pos{}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() {
					names[declName(d)] = d.Pos()
					if d.Recv == nil {
						funcs[d.Name.Name] = true
					}
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						names[s.Name.Name] = s.Pos()
					case *ast.ValueSpec:
						for _, n := range s.Names {
							names[n.Name] = n.Pos()
							if d.Tok == token.CONST {
								consts[n.Name] = true
							}
						}
					}
				}
			}
		}
	}

	out := map[string]string{}
	add := func(pos token.Pos, name, why string) {
		key := pkg + "." + name
		if _, seen := out[key]; !seen && ast.IsExported(lastPart(name)) && exportedRecv(name) {
			out[key] = fset.Position(pos).String() + ": " + why
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				name := declName(d)
				if deprecated(d.Doc) {
					add(d.Pos(), name, "its doc says Deprecated")
				} else if to, ok := forwardsTo(d, funcs); ok {
					add(d.Pos(), name, "it only passes its arguments on to "+to)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						doc := s.Doc
						if doc == nil && len(d.Specs) == 1 {
							doc = d.Doc
						}
						switch {
						case deprecated(doc):
							add(s.Pos(), s.Name.Name, "its doc says Deprecated")
						case s.Assign.IsValid():
							add(s.Pos(), s.Name.Name, "it is a type alias")
						}
						if st, ok := s.Type.(*ast.StructType); ok {
							for _, fld := range st.Fields.List {
								for _, n := range fld.Names {
									if deprecated(fld.Doc) || deprecated(fld.Comment) {
										add(n.Pos(), s.Name.Name+"."+n.Name, "its doc says Deprecated")
									}
								}
							}
						}
					case *ast.ValueSpec:
						doc := s.Doc
						if doc == nil && len(d.Specs) == 1 {
							doc = d.Doc
						}
						for i, n := range s.Names {
							switch {
							case deprecated(doc) || deprecated(s.Comment):
								add(n.Pos(), n.Name, "its doc says Deprecated")
							case i < len(s.Values):
								// A var copying another var's value is a
								// default, not a second name; a var holding a
								// function or a const naming a const is.
								if id, ok := s.Values[i].(*ast.Ident); ok && id.IsExported() &&
									(funcs[id.Name] || d.Tok == token.CONST && consts[id.Name]) {
									add(n.Pos(), n.Name, "it is another name for "+id.Name)
								}
							}
						}
					}
				}
			}
		}
	}
	for name := range names {
		if m := variantSuffix.FindStringSubmatch(lastPart(name)); m != nil {
			base := strings.TrimSuffix(name, m[2])
			if _, ok := names[base]; ok {
				add(names[name], name, "it is a variant of "+base+" (use one name and change it in place)")
			}
		}
	}
	return out
}

// declName is a function's name, or Type.Method for a method.
func declName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	typ := d.Recv.List[0].Type
	if st, ok := typ.(*ast.StarExpr); ok {
		typ = st.X
	}
	switch tt := typ.(type) {
	case *ast.IndexExpr:
		typ = tt.X
	case *ast.IndexListExpr:
		typ = tt.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

func lastPart(name string) string { return name[strings.LastIndex(name, ".")+1:] }

// exportedRecv reports whether a method's type is part of the API; a method
// on an unexported type is not.
func exportedRecv(name string) bool {
	typ, _, ok := strings.Cut(name, ".")
	return !ok || ast.IsExported(typ)
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

// forwardsTo reports whether d's whole body calls another exported function
// of the package, or another exported method on its own receiver, with
// exactly its own parameters in order: two names for one thing.
func forwardsTo(d *ast.FuncDecl, funcs map[string]bool) (string, bool) {
	if d.Body == nil || len(d.Body.List) != 1 {
		return "", false
	}
	var call *ast.CallExpr
	switch s := d.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 1 {
			call, _ = s.Results[0].(*ast.CallExpr)
		}
	case *ast.ExprStmt:
		call, _ = s.X.(*ast.CallExpr)
	}
	if call == nil {
		return "", false
	}
	var target string
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		if d.Recv != nil || !funcs[fn.Name] {
			return "", false
		}
		target = fn.Name
	case *ast.SelectorExpr:
		recv, ok := fn.X.(*ast.Ident)
		if d.Recv == nil || len(d.Recv.List) == 0 || len(d.Recv.List[0].Names) == 0 ||
			!ok || recv.Name != d.Recv.List[0].Names[0].Name || !fn.Sel.IsExported() {
			return "", false
		}
		target = fn.Sel.Name
	default:
		return "", false
	}
	if target == d.Name.Name {
		return "", false
	}
	var params []string
	variadic := false
	for _, f := range d.Type.Params.List {
		_, variadic = f.Type.(*ast.Ellipsis)
		for _, n := range f.Names {
			params = append(params, n.Name)
		}
	}
	if len(call.Args) != len(params) || call.Ellipsis.IsValid() != (variadic && len(params) > 0) {
		return "", false
	}
	for i, a := range call.Args {
		if id, ok := a.(*ast.Ident); !ok || id.Name != params[i] {
			return "", false
		}
	}
	return target, true
}

// TestShimDetector checks findShims on code it must flag and code it must
// leave alone, so a quiet TestNoCompatShims means something.
func TestShimDetector(t *testing.T) {
	dir := t.TempDir()
	src := `package fake

type Font struct{}
type FontOld = Font // alias

// Old is gone.
//
// Deprecated: use New.
func Old() {}

func Load(name string) *Font { return nil }
func LoadFont(name string) *Font { return Load(name) }
func load(name string) *Font { return Load(name) }
func Open(name string) *Font { return load(name) }
func Join(parts ...string) string { return Concat(parts...) }
func Concat(parts ...string) string { return "" }
var Parse = Load
const Max = 3
const Limit = Max
var Default = Fallback
var Fallback = 2

func (f Font) Width(s string) int { return f.Measure(s) }
func (f Font) Measure(s string) int { return len(s) }
func (f Font) Height(s string) int { return f.Measure(s + "x") }
func (f Font) DrawV2() {}
func (f Font) Draw() {}

type Opts struct {
	// Size is ignored.
	//
	// Deprecated: set Scale.
	Size int
	Scale int
}
`
	if err := os.WriteFile(dir+"/fake.go", []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(maps.Keys(findShims(t, dir, "fake")))
	want := []string{
		"fake.Font.DrawV2", "fake.Font.Width", "fake.FontOld", "fake.Join", "fake.Limit",
		"fake.LoadFont", "fake.Old", "fake.Opts.Size", "fake.Parse",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findShims flagged\n  %v\nwant\n  %v", got, want)
	}
}
