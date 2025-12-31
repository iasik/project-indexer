// Package chunker provides Go-specific code chunking using AST parsing.
// It extracts functions, methods, and type definitions as individual chunks,
// merging small helper functions into their parent scope.
package chunker

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// GoChunker implements function-level chunking for Go source code.
type GoChunker struct {
	config ChunkingConfig
}

// NewGoChunker creates a new Go code chunker.
func NewGoChunker(cfg ChunkingConfig) *GoChunker {
	return &GoChunker{config: cfg}
}

// Name returns the chunker strategy name.
func (g *GoChunker) Name() string {
	return "function"
}

// goSymbol represents an extracted Go symbol (function, type, etc.).
type goSymbol struct {
	name       string
	symbolType string
	startLine  int
	endLine    int
	content    string
	tokens     int
}

// Chunk splits Go source code into function/type-level chunks.
func (g *GoChunker) Chunk(content []byte, metadata FileMetadata) ([]Chunk, error) {
	// Parse Go source
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, metadata.FilePath, content, parser.ParseComments)
	if err != nil {
		// If parsing fails, fall back to file-level chunking
		return g.chunkAsFile(content, metadata), nil
	}

	// Extract module/package name
	module := metadata.Module
	if module == "" && file.Name != nil {
		module = file.Name.Name
	}

	// Extract all symbols
	symbols := g.extractSymbols(fset, file, string(content))

	if len(symbols) == 0 {
		return g.chunkAsFile(content, metadata), nil
	}

	// Sort by start line
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].startLine < symbols[j].startLine
	})

	// Merge small symbols if enabled
	if g.config.MergeSmallChunks {
		symbols = g.mergeSmallSymbols(symbols)
	}

	// Convert symbols to chunks
	chunks := make([]Chunk, 0, len(symbols))
	for _, sym := range symbols {
		contentHash := HashContent(sym.content)
		chunk := Chunk{
			ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, sym.name, contentHash),
			Content:     sym.content,
			Symbol:      sym.name,
			SymbolType:  sym.symbolType,
			StartLine:   sym.startLine,
			EndLine:     sym.endLine,
			TokenCount:  sym.tokens,
			ContentHash: contentHash,
			FilePath:    metadata.FilePath,
			Language:    "go",
			Module:      module,
			ProjectID:   metadata.ProjectID,
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// extractSymbols extracts all function and type declarations from the AST.
func (g *GoChunker) extractSymbols(fset *token.FileSet, file *ast.File, source string) []goSymbol {
	lines := strings.Split(source, "\n")
	symbols := make([]goSymbol, 0)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := g.extractFunction(fset, d, lines)
			if sym != nil {
				symbols = append(symbols, *sym)
			}

		case *ast.GenDecl:
			// Extract type declarations
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						sym := g.extractType(fset, d, ts, lines)
						if sym != nil {
							symbols = append(symbols, *sym)
						}
					}
				}
			}
		}
	}

	return symbols
}

// extractFunction extracts a function declaration.
func (g *GoChunker) extractFunction(fset *token.FileSet, fn *ast.FuncDecl, lines []string) *goSymbol {
	startLine := fset.Position(fn.Pos()).Line
	endLine := fset.Position(fn.End()).Line

	// Capture doc comments if present
	if fn.Doc != nil {
		docStart := fset.Position(fn.Doc.Pos()).Line
		if docStart < startLine {
			startLine = docStart
		}
	}

	content := extractLines(lines, startLine, endLine)
	name := fn.Name.Name

	// Determine symbol type (function or method)
	symbolType := "function"
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		symbolType = "method"
		// Include receiver type in name for methods
		if recv := fn.Recv.List[0]; recv.Type != nil {
			recvType := extractReceiverType(recv.Type)
			if recvType != "" {
				name = fmt.Sprintf("%s.%s", recvType, fn.Name.Name)
			}
		}
	}

	return &goSymbol{
		name:       name,
		symbolType: symbolType,
		startLine:  startLine,
		endLine:    endLine,
		content:    content,
		tokens:     EstimateTokens(content),
	}
}

// extractType extracts a type declaration (struct, interface, etc.).
func (g *GoChunker) extractType(fset *token.FileSet, genDecl *ast.GenDecl, ts *ast.TypeSpec, lines []string) *goSymbol {
	startLine := fset.Position(genDecl.Pos()).Line
	endLine := fset.Position(genDecl.End()).Line

	// Capture doc comments
	if genDecl.Doc != nil {
		docStart := fset.Position(genDecl.Doc.Pos()).Line
		if docStart < startLine {
			startLine = docStart
		}
	}

	content := extractLines(lines, startLine, endLine)

	symbolType := "type"
	switch ts.Type.(type) {
	case *ast.StructType:
		symbolType = "struct"
	case *ast.InterfaceType:
		symbolType = "interface"
	}

	return &goSymbol{
		name:       ts.Name.Name,
		symbolType: symbolType,
		startLine:  startLine,
		endLine:    endLine,
		content:    content,
		tokens:     EstimateTokens(content),
	}
}

// extractReceiverType extracts the receiver type name from a method.
func extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// extractLines extracts lines from source (1-indexed, inclusive).
func extractLines(lines []string, start, end int) string {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// mergeSmallSymbols merges symbols below MinTokens into adjacent larger symbols.
func (g *GoChunker) mergeSmallSymbols(symbols []goSymbol) []goSymbol {
	if len(symbols) <= 1 {
		return symbols
	}

	result := make([]goSymbol, 0, len(symbols))
	pendingSmall := make([]goSymbol, 0)

	for _, sym := range symbols {
		if sym.tokens < g.config.MinTokens {
			// Accumulate small symbols
			pendingSmall = append(pendingSmall, sym)
		} else {
			// Found a large symbol, attach pending small ones
			if len(pendingSmall) > 0 {
				sym = g.mergeInto(sym, pendingSmall)
				pendingSmall = pendingSmall[:0]
			}
			result = append(result, sym)
		}
	}

	// Handle remaining small symbols
	if len(pendingSmall) > 0 {
		if len(result) > 0 {
			// Merge into last large symbol
			last := &result[len(result)-1]
			*last = g.mergeInto(*last, pendingSmall)
		} else {
			// All symbols are small, combine them all
			merged := g.combineSymbols(pendingSmall)
			result = append(result, merged)
		}
	}

	return result
}

// mergeInto merges small symbols into a larger symbol.
func (g *GoChunker) mergeInto(target goSymbol, small []goSymbol) goSymbol {
	// Combine content
	allContent := []string{target.content}
	names := []string{target.name}

	for _, s := range small {
		allContent = append(allContent, s.content)
		names = append(names, s.name)
	}

	// Update target
	target.content = strings.Join(allContent, "\n\n")
	target.tokens = EstimateTokens(target.content)

	// Update line range
	for _, s := range small {
		if s.startLine < target.startLine {
			target.startLine = s.startLine
		}
		if s.endLine > target.endLine {
			target.endLine = s.endLine
		}
	}

	// Update name to indicate merged content
	if len(names) > 1 {
		target.name = fmt.Sprintf("%s+%d", target.name, len(small))
	}

	return target
}

// combineSymbols combines multiple small symbols into one.
func (g *GoChunker) combineSymbols(symbols []goSymbol) goSymbol {
	if len(symbols) == 0 {
		return goSymbol{}
	}

	var content []string
	var names []string
	startLine := symbols[0].startLine
	endLine := symbols[0].endLine

	for _, s := range symbols {
		content = append(content, s.content)
		names = append(names, s.name)
		if s.startLine < startLine {
			startLine = s.startLine
		}
		if s.endLine > endLine {
			endLine = s.endLine
		}
	}

	combinedContent := strings.Join(content, "\n\n")

	return goSymbol{
		name:       strings.Join(names, "+"),
		symbolType: "combined",
		startLine:  startLine,
		endLine:    endLine,
		content:    combinedContent,
		tokens:     EstimateTokens(combinedContent),
	}
}

// chunkAsFile returns the entire file as a single chunk.
func (g *GoChunker) chunkAsFile(content []byte, metadata FileMetadata) []Chunk {
	contentStr := string(content)
	contentHash := HashContent(contentStr)

	// Extract package name from file path
	symbol := filepath.Base(metadata.FilePath)

	return []Chunk{{
		ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, symbol, contentHash),
		Content:     contentStr,
		Symbol:      symbol,
		SymbolType:  "file",
		StartLine:   1,
		EndLine:     strings.Count(contentStr, "\n") + 1,
		TokenCount:  EstimateTokens(contentStr),
		ContentHash: contentHash,
		FilePath:    metadata.FilePath,
		Language:    "go",
		Module:      metadata.Module,
		ProjectID:   metadata.ProjectID,
	}}
}
