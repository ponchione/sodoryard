package graph

import (
	"fmt"
	"go/ast"
	"go/token"
	goTypes "go/types"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel/goload"
	"golang.org/x/tools/go/packages"
)

// GoAnalyzer extracts symbols and edges from a Go module using
// go/packages and go/types for fully type-checked resolution.
type GoAnalyzer struct {
	fset       *token.FileSet
	pkgsByFile map[string]*packages.Package // abs file path → package
	allIfaces  []goload.InterfaceInfo
	modulePath string
	rootDir    string
}

// NewGoAnalyzer loads all Go packages under rootDir and builds lookup indexes.
func NewGoAnalyzer(rootDir string) (*GoAnalyzer, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	modulePath, err := readModulePath(absRoot)
	if err != nil {
		return nil, fmt.Errorf("read module path: %w", err)
	}

	loaded, err := goload.Load(absRoot, modulePath)
	if err != nil {
		return nil, err
	}

	slog.Info("GoAnalyzer initialized",
		"module", modulePath,
		"files", len(loaded.ByFile),
		"interfaces", len(loaded.Interfaces),
	)

	return &GoAnalyzer{
		fset:       loaded.FileSet,
		pkgsByFile: loaded.ByFile,
		allIfaces:  loaded.Interfaces,
		modulePath: modulePath,
		rootDir:    absRoot,
	}, nil
}

// readModulePath reads the module path from go.mod.
func readModulePath(rootDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module directive found in go.mod")
}

// symbolID creates a deterministic symbol ID.
// Format: {language}:{package}:{kind}:{name}
func symbolID(lang, pkg, kind, name string) string {
	return lang + ":" + pkg + ":" + kind + ":" + name
}

// receiverTypeName extracts the type name from a method receiver field list.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || recv.NumFields() == 0 {
		return ""
	}
	typ := recv.List[0].Type
	// Unwrap pointer: *T → T
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if ident, ok := typ.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// relPath converts an absolute file path to a project-relative path.
func (a *GoAnalyzer) relPath(absPath string) string {
	rel, err := filepath.Rel(a.rootDir, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// isInModule returns true if the package path is within the project module.
func (a *GoAnalyzer) isInModule(pkgPath string) bool {
	return strings.HasPrefix(pkgPath, a.modulePath)
}

// findASTFile locates the ast.File in a package's syntax that corresponds to absPath.
func (a *GoAnalyzer) findASTFile(pkg *packages.Package, absPath string) *ast.File {
	for _, f := range pkg.Syntax {
		pos := a.fset.Position(f.Pos())
		fAbs, _ := filepath.Abs(pos.Filename)
		if fAbs == absPath {
			return f
		}
	}
	return nil
}

// Analyze extracts all symbols and edges from the Go module.
func (a *GoAnalyzer) Analyze() (*AnalysisResult, error) {
	symbols := a.extractSymbols()
	edges, boundaries := a.extractEdges(symbols)

	slog.Info("Go analysis complete",
		"symbols", len(symbols),
		"edges", len(edges),
		"boundary_symbols", len(boundaries),
	)

	return &AnalysisResult{
		Symbols:         symbols,
		Edges:           edges,
		BoundarySymbols: boundaries,
	}, nil
}

type goModuleFile struct {
	absPath string
	pkg     *packages.Package
	astFile *ast.File
	relFile string
}

func (a *GoAnalyzer) forEachModuleFile(fn func(goModuleFile)) {
	for absPath, pkg := range a.pkgsByFile {
		if !a.isInModule(pkg.PkgPath) {
			continue
		}
		astFile := a.findASTFile(pkg, absPath)
		if astFile == nil {
			continue
		}
		fn(goModuleFile{
			absPath: absPath,
			pkg:     pkg,
			astFile: astFile,
			relFile: a.relPath(absPath),
		})
	}
}

// extractSymbols walks all files in the module and extracts symbols.
func (a *GoAnalyzer) extractSymbols() []Symbol {
	var symbols []Symbol
	seen := make(map[string]bool)

	a.forEachModuleFile(func(file goModuleFile) {
		for _, decl := range file.astFile.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				sym := a.extractFuncSymbol(d, file.pkg, file.relFile)
				if sym != nil && !seen[sym.ID] {
					seen[sym.ID] = true
					symbols = append(symbols, *sym)
				}
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, sym := range a.extractTypeSymbols(d, file.pkg, file.relFile) {
						if !seen[sym.ID] {
							seen[sym.ID] = true
							symbols = append(symbols, sym)
						}
					}
				}
			}
		}
	})

	return symbols
}

// extractFuncSymbol extracts a Symbol from a function/method declaration.
func (a *GoAnalyzer) extractFuncSymbol(decl *ast.FuncDecl, pkg *packages.Package, relFile string) *Symbol {
	name := decl.Name.Name
	if name == "" {
		return nil
	}

	kind := "function"
	receiver := ""
	qualName := name

	if decl.Recv != nil && decl.Recv.NumFields() > 0 {
		kind = "method"
		receiver = receiverTypeName(decl.Recv)
		if receiver != "" {
			qualName = receiver + "." + name
		}
	}

	startPos := a.fset.Position(decl.Pos())
	endPos := a.fset.Position(decl.End())

	sig := a.buildFuncSignature(decl, receiver, name, pkg)

	return &Symbol{
		ID:        symbolID("go", pkg.PkgPath, kind, qualName),
		Name:      name,
		Kind:      kind,
		Language:  "go",
		Package:   pkg.PkgPath,
		FilePath:  relFile,
		LineStart: startPos.Line,
		LineEnd:   endPos.Line,
		Signature: sig,
		Exported:  ast.IsExported(name),
		Receiver:  receiver,
	}
}

// buildFuncSignature constructs a human-readable function signature.
func (a *GoAnalyzer) buildFuncSignature(decl *ast.FuncDecl, receiver, name string, _ *packages.Package) string {
	sig := "func "
	if receiver != "" {
		sig = fmt.Sprintf("func (%s) ", receiver)
	}
	sig += name
	if decl.Type.Params != nil {
		sig += "(" + a.formatFieldList(decl.Type.Params) + ")"
	} else {
		sig += "()"
	}
	if decl.Type.Results != nil && decl.Type.Results.NumFields() > 0 {
		results := a.formatFieldList(decl.Type.Results)
		if decl.Type.Results.NumFields() == 1 && !strings.Contains(results, " ") {
			sig += " " + results
		} else {
			sig += " (" + results + ")"
		}
	}
	return sig
}

// formatFieldList formats a field list for signature display.
func (a *GoAnalyzer) formatFieldList(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	var parts []string
	for _, field := range fl.List {
		typeStr := goTypes.ExprString(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typeStr)
		} else {
			for _, name := range field.Names {
				parts = append(parts, name.Name+" "+typeStr)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// extractTypeSymbols extracts Symbol values from type declarations.
func (a *GoAnalyzer) extractTypeSymbols(decl *ast.GenDecl, pkg *packages.Package, relFile string) []Symbol {
	var symbols []Symbol

	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || ts.Name.Name == "" {
			continue
		}

		name := ts.Name.Name
		kind := "type"

		switch ts.Type.(type) {
		case *ast.InterfaceType:
			kind = "interface"
		}

		startNode := ast.Node(decl)
		if len(decl.Specs) > 1 {
			startNode = ts
		}

		startPos := a.fset.Position(startNode.Pos())
		endPos := a.fset.Position(startNode.End())

		sig := "type " + name
		switch ts.Type.(type) {
		case *ast.InterfaceType:
			sig += " interface"
		case *ast.StructType:
			sig += " struct"
		}

		symbols = append(symbols, Symbol{
			ID:        symbolID("go", pkg.PkgPath, kind, name),
			Name:      name,
			Kind:      kind,
			Language:  "go",
			Package:   pkg.PkgPath,
			FilePath:  relFile,
			LineStart: startPos.Line,
			LineEnd:   endPos.Line,
			Signature: sig,
			Exported:  ast.IsExported(name),
		})
	}

	return symbols
}

// extractEdges walks all files in the module and resolves edges.
func (a *GoAnalyzer) extractEdges(_ []Symbol) ([]Edge, []BoundarySymbol) {
	var edges []Edge
	boundaryMap := make(map[string]BoundarySymbol)

	// Track dedup for embed and implements edges across files.
	seenEmbeds := make(map[string]bool)
	seenImplements := make(map[string]bool)

	a.forEachModuleFile(func(file goModuleFile) {
		// Per-file: embeds and implements edges (type-level, not function-level).
		embedEdges := a.extractEmbedEdges(file.astFile, file.pkg)
		for _, e := range embedEdges {
			key := e.SourceID + "->" + e.TargetID
			if !seenEmbeds[key] {
				seenEmbeds[key] = true
				edges = append(edges, e)
			}
		}

		implEdges := a.extractImplementsEdges(file.astFile, file.pkg)
		for _, e := range implEdges {
			key := e.SourceID + "->" + e.TargetID
			if !seenImplements[key] {
				seenImplements[key] = true
				edges = append(edges, e)
			}
		}

		// Per-function: call edges and import edges.
		for _, decl := range file.astFile.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}

			sourceID := a.funcDeclSymbolID(fd, file.pkg)
			if sourceID == "" {
				continue
			}

			callEdges, bounds := a.extractCallEdges(fd.Body, file.pkg, sourceID)
			edges = append(edges, callEdges...)
			for _, b := range bounds {
				boundaryMap[b.ID] = b
			}

			importEdges, importBounds := a.extractImportEdges(file.astFile, file.pkg, sourceID)
			edges = append(edges, importEdges...)
			for _, b := range importBounds {
				boundaryMap[b.ID] = b
			}
		}
	})

	var boundaries []BoundarySymbol
	for _, b := range boundaryMap {
		boundaries = append(boundaries, b)
	}
	return edges, boundaries
}

// funcDeclSymbolID returns the symbol ID for a function declaration.
func (a *GoAnalyzer) funcDeclSymbolID(decl *ast.FuncDecl, pkg *packages.Package) string {
	name := decl.Name.Name
	if name == "" {
		return ""
	}
	kind := "function"
	qualName := name
	if decl.Recv != nil && decl.Recv.NumFields() > 0 {
		kind = "method"
		recv := receiverTypeName(decl.Recv)
		if recv != "" {
			qualName = recv + "." + name
		}
	}
	return symbolID("go", pkg.PkgPath, kind, qualName)
}

// extractCallEdges walks a function body for call expressions.
func (a *GoAnalyzer) extractCallEdges(body *ast.BlockStmt, pkg *packages.Package, sourceID string) ([]Edge, []BoundarySymbol) {
	if body == nil || pkg.TypesInfo == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var edges []Edge
	var boundaries []BoundarySymbol

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var targetPkg, targetName, targetKind string

		switch fn := call.Fun.(type) {
		case *ast.Ident:
			targetPkg, targetName, targetKind = a.resolveIdentCall(fn, pkg)

		case *ast.SelectorExpr:
			targetPkg, targetName, targetKind = a.resolveSelectorCall(fn, pkg)
		}

		if targetName == "" || targetPkg == "" {
			return true
		}

		targetID := symbolID("go", targetPkg, targetKind, targetName)

		if seen[targetID] {
			return true
		}
		seen[targetID] = true

		sourceLine := a.fset.Position(call.Pos()).Line

		edges = append(edges, Edge{
			SourceID:   sourceID,
			TargetID:   targetID,
			EdgeType:   "CALLS",
			Confidence: 1.0,
			SourceLine: sourceLine,
		})

		if !a.isInModule(targetPkg) {
			boundaries = append(boundaries, BoundarySymbol{
				ID:       targetID,
				Name:     targetName,
				Kind:     targetKind,
				Language: "go",
				Package:  targetPkg,
			})
		}

		return true
	})

	return edges, boundaries
}

// resolveIdentCall resolves a simple identifier call (e.g., doSomething()).
func (a *GoAnalyzer) resolveIdentCall(ident *ast.Ident, pkg *packages.Package) (pkgPath, name, kind string) {
	obj, ok := pkg.TypesInfo.Uses[ident]
	if !ok {
		return "", "", ""
	}

	fn, ok := obj.(*goTypes.Func)
	if !ok {
		return "", "", ""
	}

	name = fn.Name()
	kind = "function"
	if fn.Pkg() != nil {
		pkgPath = fn.Pkg().Path()
	}
	return pkgPath, name, kind
}

// resolveSelectorCall resolves a selector expression call (e.g., pkg.Func() or obj.Method()).
func (a *GoAnalyzer) resolveSelectorCall(sel *ast.SelectorExpr, pkg *packages.Package) (pkgPath, name, kind string) {
	// First try: is X a package name? e.g., fmt.Println()
	if xIdent, ok := sel.X.(*ast.Ident); ok {
		if xObj, ok := pkg.TypesInfo.Uses[xIdent]; ok {
			if pkgName, ok := xObj.(*goTypes.PkgName); ok {
				return pkgName.Imported().Path(), sel.Sel.Name, "function"
			}
		}
	}

	// Second: resolve as method call.
	selObj, ok := pkg.TypesInfo.Uses[sel.Sel]
	if !ok {
		return "", "", ""
	}

	fn, ok := selObj.(*goTypes.Func)
	if !ok {
		return "", "", ""
	}

	if fn.Pkg() != nil {
		pkgPath = fn.Pkg().Path()
	}

	kind = "method"
	name = fn.Name()

	// Get receiver type for method calls to build qualified name.
	sig := fn.Type().(*goTypes.Signature)
	if sig.Recv() != nil {
		recvType := sig.Recv().Type()
		if ptr, ok := recvType.(*goTypes.Pointer); ok {
			recvType = ptr.Elem()
		}
		if named, ok := recvType.(*goTypes.Named); ok {
			name = named.Obj().Name() + "." + fn.Name()
		}
	}

	return pkgPath, name, kind
}

// extractImplementsEdges checks interface satisfaction for types in the file.
func (a *GoAnalyzer) extractImplementsEdges(file *ast.File, pkg *packages.Package) []Edge {
	if pkg.Types == nil {
		return nil
	}

	var edges []Edge

	walkGoTypeSpecs(file, func(ts *ast.TypeSpec) {
		obj := pkg.Types.Scope().Lookup(ts.Name.Name)
		if obj == nil {
			return
		}

		if _, ok := obj.Type().Underlying().(*goTypes.Interface); ok {
			return
		}

		typ := obj.Type()
		ptrTyp := goTypes.NewPointer(typ)

		for _, iface := range a.allIfaces {
			if goTypes.Implements(typ, iface.Type) || goTypes.Implements(ptrTyp, iface.Type) {
				sourceID := symbolID("go", pkg.PkgPath, "type", ts.Name.Name)
				targetID := symbolID("go", iface.PkgPath, "interface", iface.Name)

				edges = append(edges, Edge{
					SourceID:   sourceID,
					TargetID:   targetID,
					EdgeType:   "IMPLEMENTS",
					Confidence: 1.0,
				})
			}
		}
	})

	return edges
}

// extractEmbedEdges detects struct embedding (anonymous fields).
func (a *GoAnalyzer) extractEmbedEdges(file *ast.File, pkg *packages.Package) []Edge {
	if pkg.Types == nil {
		return nil
	}

	var edges []Edge

	walkGoTypeSpecs(file, func(ts *ast.TypeSpec) {
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			return
		}

		sourceID := symbolID("go", pkg.PkgPath, "type", ts.Name.Name)

		for _, field := range st.Fields.List {
			if len(field.Names) > 0 {
				continue // named field, not an embedding
			}

			// Resolve the embedded type.
			typ := field.Type
			if star, ok := typ.(*ast.StarExpr); ok {
				typ = star.X
			}

			var embedPkg, embedName string
			switch t := typ.(type) {
			case *ast.Ident:
				embedName = t.Name
				embedPkg = pkg.PkgPath
			case *ast.SelectorExpr:
				if xIdent, ok := t.X.(*ast.Ident); ok {
					if obj, ok := pkg.TypesInfo.Uses[xIdent]; ok {
						if pkgName, ok := obj.(*goTypes.PkgName); ok {
							embedPkg = pkgName.Imported().Path()
							embedName = t.Sel.Name
						}
					}
				}
			}

			if embedName == "" {
				continue
			}

			targetID := symbolID("go", embedPkg, "type", embedName)
			edges = append(edges, Edge{
				SourceID:   sourceID,
				TargetID:   targetID,
				EdgeType:   "EMBEDS",
				Confidence: 1.0,
				SourceLine: a.fset.Position(field.Pos()).Line,
			})
		}
	})

	return edges
}

func walkGoTypeSpecs(file *ast.File, visit func(*ast.TypeSpec)) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if ok {
				visit(ts)
			}
		}
	}
}

// extractImportEdges creates IMPORTS edges for each import in the file.
func (a *GoAnalyzer) extractImportEdges(file *ast.File, _ *packages.Package, sourceID string) ([]Edge, []BoundarySymbol) {
	var edges []Edge
	var boundaries []BoundarySymbol

	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		targetID := symbolID("go", importPath, "module", importPath)
		sourceLine := a.fset.Position(imp.Pos()).Line

		edges = append(edges, Edge{
			SourceID:   sourceID,
			TargetID:   targetID,
			EdgeType:   "IMPORTS",
			Confidence: 1.0,
			SourceLine: sourceLine,
		})

		if !a.isInModule(importPath) {
			boundaries = append(boundaries, BoundarySymbol{
				ID:       targetID,
				Name:     importPath,
				Kind:     "module",
				Language: "go",
				Package:  importPath,
			})
		}
	}

	return edges, boundaries
}
