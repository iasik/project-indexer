// Package chunker provides TypeScript/JavaScript code chunking using regex-based parsing.
// It extracts functions, classes, interfaces, types, and React components as individual chunks.
package chunker

import (
	"regexp"
	"sort"
	"strings"
)

// TypeScriptChunker implements function/class-level chunking for TypeScript/JavaScript.
type TypeScriptChunker struct {
	config ChunkingConfig
}

// NewTypeScriptChunker creates a new TypeScript/JavaScript chunker.
func NewTypeScriptChunker(cfg ChunkingConfig) *TypeScriptChunker {
	return &TypeScriptChunker{config: cfg}
}

// Name returns the chunker strategy name.
func (t *TypeScriptChunker) Name() string {
	return "typescript"
}

// tsSymbol represents an extracted TypeScript symbol.
type tsSymbol struct {
	name       string
	symbolType string
	startLine  int
	endLine    int
	content    string
	tokens     int
}

// Regex patterns for TypeScript/JavaScript symbol extraction
var (
	// Named function: function foo(...) { or export function foo(...) { or async function foo(...)
	tsFunctionPattern = regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*(?:<[^>]*>)?\s*\(`)

	// Arrow function: const foo = (...) => or export const foo = (...) =>
	tsArrowFuncPattern = regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*(?::\s*[^=]+)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[^=])\s*=>`)

	// Class: class Foo { or export class Foo extends Bar
	tsClassPattern = regexp.MustCompile(`(?m)^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)(?:\s+extends\s+\w+)?(?:\s+implements\s+[\w,\s<>]+)?\s*\{`)

	// Interface: interface Foo { or export interface Foo
	tsInterfacePattern = regexp.MustCompile(`(?m)^(?:export\s+)?interface\s+(\w+)(?:\s*<[^>]+>)?(?:\s+extends\s+[\w,\s<>]+)?\s*\{`)

	// Type alias: type Foo = or export type Foo =
	tsTypePattern = regexp.MustCompile(`(?m)^(?:export\s+)?type\s+(\w+)(?:\s*<[^>]+>)?\s*=`)

	// Enum: enum Foo { or export enum Foo {
	tsEnumPattern = regexp.MustCompile(`(?m)^(?:export\s+)?(?:const\s+)?enum\s+(\w+)\s*\{`)

	// React component patterns (PascalCase function returning JSX)
	tsReactFCPattern = regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|function)\s+([A-Z]\w+)\s*(?::\s*(?:React\.)?(?:FC|FunctionComponent))?`)

	// Export default
	tsExportDefaultPattern = regexp.MustCompile(`(?m)^export\s+default\s+(?:async\s+)?(?:function|class)\s+(\w+)?`)

	// Method inside class (for context)
	tsMethodPattern = regexp.MustCompile(`(?m)^\s+(?:public|private|protected|static|async|readonly|\s)*(\w+)\s*(?:<[^>]*>)?\s*\([^)]*\)\s*(?::\s*[^{]+)?\s*\{`)

	// JSDoc comment
	tsJSDocPattern = regexp.MustCompile(`(?s)/\*\*.*?\*/`)

	// Single-line comment
	tsSingleCommentPattern = regexp.MustCompile(`(?m)^[\t ]*//.*$`)
)

// symbolMatch holds a regex match with metadata.
type symbolMatch struct {
	name       string
	symbolType string
	lineNum    int
	matchStart int
	matchEnd   int
}

// Chunk splits TypeScript/JavaScript source code into function/class-level chunks.
func (t *TypeScriptChunker) Chunk(content []byte, metadata FileMetadata) ([]Chunk, error) {
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	// Find all symbol declarations
	matches := t.findAllSymbols(contentStr, lines)

	if len(matches) == 0 {
		// Fall back to generic chunking if no symbols found
		return t.chunkAsFile(content, metadata), nil
	}

	// Sort by line number
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].lineNum < matches[j].lineNum
	})

	// Extract symbols with their boundaries
	symbols := t.extractSymbolBoundaries(contentStr, lines, matches)

	// Merge small symbols if enabled
	if t.config.MergeSmallChunks {
		symbols = t.mergeSmallSymbols(symbols)
	}

	// Convert to chunks
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
			Language:    metadata.Language,
			Module:      metadata.Module,
			ProjectID:   metadata.ProjectID,
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// findAllSymbols finds all symbol declarations in the content.
func (t *TypeScriptChunker) findAllSymbols(content string, lines []string) []symbolMatch {
	matches := make([]symbolMatch, 0)

	// Helper to add matches from a pattern
	addMatches := func(pattern *regexp.Regexp, symbolType string) {
		for _, loc := range pattern.FindAllStringSubmatchIndex(content, -1) {
			if len(loc) >= 4 {
				// loc[0:2] is full match, loc[2:4] is first capture group
				name := ""
				if loc[2] >= 0 && loc[3] >= 0 {
					name = content[loc[2]:loc[3]]
				}
				if name == "" {
					continue
				}

				lineNum := strings.Count(content[:loc[0]], "\n") + 1
				matches = append(matches, symbolMatch{
					name:       name,
					symbolType: symbolType,
					lineNum:    lineNum,
					matchStart: loc[0],
					matchEnd:   loc[1],
				})
			}
		}
	}

	// Find all symbol types
	addMatches(tsClassPattern, "class")
	addMatches(tsInterfacePattern, "interface")
	addMatches(tsEnumPattern, "enum")
	addMatches(tsTypePattern, "type")
	addMatches(tsFunctionPattern, "function")
	addMatches(tsArrowFuncPattern, "arrow_function")
	addMatches(tsExportDefaultPattern, "export_default")

	// Deduplicate by line number (prefer more specific types)
	return t.deduplicateMatches(matches)
}

// deduplicateMatches removes duplicate matches on the same line.
func (t *TypeScriptChunker) deduplicateMatches(matches []symbolMatch) []symbolMatch {
	// Priority: class > interface > enum > type > function > arrow_function
	priority := map[string]int{
		"class":          1,
		"interface":      2,
		"enum":           3,
		"type":           4,
		"function":       5,
		"arrow_function": 6,
		"export_default": 7,
	}

	byLine := make(map[int]symbolMatch)
	for _, m := range matches {
		if existing, ok := byLine[m.lineNum]; ok {
			// Keep higher priority (lower number)
			if priority[m.symbolType] < priority[existing.symbolType] {
				byLine[m.lineNum] = m
			}
		} else {
			byLine[m.lineNum] = m
		}
	}

	result := make([]symbolMatch, 0, len(byLine))
	for _, m := range byLine {
		result = append(result, m)
	}
	return result
}

// extractSymbolBoundaries finds the start and end of each symbol using brace matching.
func (t *TypeScriptChunker) extractSymbolBoundaries(content string, lines []string, matches []symbolMatch) []tsSymbol {
	symbols := make([]tsSymbol, 0, len(matches))

	for i, m := range matches {
		startLine := m.lineNum

		// Look for preceding JSDoc comment
		if startLine > 1 {
			startLine = t.findPrecedingComment(lines, startLine)
		}

		// Find end line using brace matching
		var endLine int
		if m.symbolType == "type" {
			// Type aliases end at semicolon or next symbol
			endLine = t.findTypeEnd(lines, m.lineNum)
		} else {
			// Classes, functions, etc. use brace matching
			endLine = t.findBraceEnd(lines, m.lineNum)
		}

		// Ensure we don't overlap with next symbol
		if i < len(matches)-1 {
			nextStart := matches[i+1].lineNum
			if endLine >= nextStart {
				endLine = nextStart - 1
			}
		}

		// Don't go past file end
		if endLine > len(lines) {
			endLine = len(lines)
		}
		if endLine < startLine {
			endLine = startLine
		}

		// Extract content
		symContent := extractLines(lines, startLine, endLine)
		tokens := EstimateTokens(symContent)

		symbols = append(symbols, tsSymbol{
			name:       m.name,
			symbolType: m.symbolType,
			startLine:  startLine,
			endLine:    endLine,
			content:    symContent,
			tokens:     tokens,
		})
	}

	return symbols
}

// findPrecedingComment finds JSDoc or line comments before a symbol.
func (t *TypeScriptChunker) findPrecedingComment(lines []string, symbolLine int) int {
	startLine := symbolLine

	for i := symbolLine - 2; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])

		// Check for JSDoc end
		if strings.HasSuffix(line, "*/") {
			// Find JSDoc start
			for j := i; j >= 0; j-- {
				if strings.Contains(lines[j], "/**") {
					return j + 1
				}
			}
		}

		// Check for single-line comment
		if strings.HasPrefix(line, "//") {
			startLine = i + 1
			continue
		}

		// Check for decorator/attribute
		if strings.HasPrefix(line, "@") {
			startLine = i + 1
			continue
		}

		// Empty line or other content - stop looking
		if line != "" {
			break
		}
	}

	return startLine
}

// findBraceEnd finds the closing brace for a block using brace matching.
func (t *TypeScriptChunker) findBraceEnd(lines []string, startLine int) int {
	depth := 0
	foundOpen := false

	for i := startLine - 1; i < len(lines); i++ {
		line := lines[i]

		// Skip strings (simple heuristic)
		inString := false
		stringChar := byte(0)

		for j := 0; j < len(line); j++ {
			ch := line[j]

			// Handle string literals (simple - doesn't handle all edge cases)
			if !inString && (ch == '"' || ch == '\'' || ch == '`') {
				inString = true
				stringChar = ch
				continue
			}
			if inString && ch == stringChar && (j == 0 || line[j-1] != '\\') {
				inString = false
				continue
			}
			if inString {
				continue
			}

			// Handle braces
			if ch == '{' {
				depth++
				foundOpen = true
			} else if ch == '}' {
				depth--
				if foundOpen && depth == 0 {
					return i + 1
				}
			}
		}
	}

	// If no matching brace found, return a reasonable end
	return startLine + 50 // Arbitrary limit
}

// findTypeEnd finds the end of a type alias (ends at semicolon or next declaration).
func (t *TypeScriptChunker) findTypeEnd(lines []string, startLine int) int {
	depth := 0

	for i := startLine - 1; i < len(lines); i++ {
		line := lines[i]

		for _, ch := range line {
			if ch == '{' || ch == '<' || ch == '(' {
				depth++
			} else if ch == '}' || ch == '>' || ch == ')' {
				depth--
			} else if ch == ';' && depth == 0 {
				return i + 1
			}
		}

		// Check if next line starts a new declaration
		if i+1 < len(lines) && depth == 0 {
			nextLine := strings.TrimSpace(lines[i+1])
			if strings.HasPrefix(nextLine, "export ") ||
				strings.HasPrefix(nextLine, "type ") ||
				strings.HasPrefix(nextLine, "interface ") ||
				strings.HasPrefix(nextLine, "class ") ||
				strings.HasPrefix(nextLine, "function ") ||
				strings.HasPrefix(nextLine, "const ") ||
				strings.HasPrefix(nextLine, "let ") {
				return i + 1
			}
		}
	}

	return startLine
}

// mergeSmallSymbols merges small symbols into adjacent larger ones.
func (t *TypeScriptChunker) mergeSmallSymbols(symbols []tsSymbol) []tsSymbol {
	if len(symbols) <= 1 {
		return symbols
	}

	result := make([]tsSymbol, 0, len(symbols))
	var pending *tsSymbol

	for i := range symbols {
		sym := symbols[i]

		if sym.tokens < t.config.MinTokens {
			if pending == nil {
				pending = &sym
			} else {
				// Merge with pending
				pending.content += "\n\n" + sym.content
				pending.endLine = sym.endLine
				pending.tokens = EstimateTokens(pending.content)
				pending.name = pending.name + "+" + sym.name
			}
		} else {
			if pending != nil {
				// Merge pending into current if total doesn't exceed max
				if pending.tokens+sym.tokens <= t.config.MaxTokens {
					sym.content = pending.content + "\n\n" + sym.content
					sym.startLine = pending.startLine
					sym.tokens = EstimateTokens(sym.content)
				} else {
					result = append(result, *pending)
				}
				pending = nil
			}
			result = append(result, sym)
		}
	}

	if pending != nil {
		result = append(result, *pending)
	}

	return result
}

// chunkAsFile creates a single chunk for the entire file.
func (t *TypeScriptChunker) chunkAsFile(content []byte, metadata FileMetadata) []Chunk {
	contentStr := string(content)
	contentHash := HashContent(contentStr)
	symbol := metadata.FilePath

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
		Language:    metadata.Language,
		Module:      metadata.Module,
		ProjectID:   metadata.ProjectID,
	}}
}
