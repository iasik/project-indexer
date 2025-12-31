// Package chunker provides PHP code chunking using regex-based parsing.
// It extracts functions, classes, methods, traits, and interfaces as individual chunks.
package chunker

import (
	"regexp"
	"sort"
	"strings"
)

// PHPChunker implements function/class-level chunking for PHP code.
type PHPChunker struct {
	config ChunkingConfig
}

// NewPHPChunker creates a new PHP chunker.
func NewPHPChunker(cfg ChunkingConfig) *PHPChunker {
	return &PHPChunker{config: cfg}
}

// Name returns the chunker strategy name.
func (p *PHPChunker) Name() string {
	return "php"
}

// phpSymbol represents an extracted PHP symbol.
type phpSymbol struct {
	name       string
	symbolType string
	startLine  int
	endLine    int
	content    string
	tokens     int
	namespace  string
}

// Regex patterns for PHP symbol extraction
var (
	// Namespace: namespace Foo\Bar;
	phpNamespacePattern = regexp.MustCompile(`(?m)^namespace\s+([\w\\]+)\s*;`)

	// Class: class Foo { or abstract class Foo extends Bar implements Baz
	phpClassPattern = regexp.MustCompile(`(?m)^(?:abstract\s+|final\s+)?class\s+(\w+)(?:\s+extends\s+[\w\\]+)?(?:\s+implements\s+[\w\\,\s]+)?\s*\{?`)

	// Interface: interface Foo { or interface Foo extends Bar
	phpInterfacePattern = regexp.MustCompile(`(?m)^interface\s+(\w+)(?:\s+extends\s+[\w\\,\s]+)?\s*\{`)

	// Trait: trait Foo {
	phpTraitPattern = regexp.MustCompile(`(?m)^trait\s+(\w+)\s*\{`)

	// Enum (PHP 8.1+): enum Foo { or enum Foo: string
	phpEnumPattern = regexp.MustCompile(`(?m)^enum\s+(\w+)(?:\s*:\s*\w+)?(?:\s+implements\s+[\w\\,\s]+)?\s*\{`)

	// Standalone function: function foo(
	phpFunctionPattern = regexp.MustCompile(`(?m)^function\s+(\w+)\s*\(`)

	// Class method with visibility: public function foo( or private static function bar(
	phpMethodPattern = regexp.MustCompile(`(?m)^\s+(?:(?:public|private|protected|static|final|abstract)\s+)+function\s+(\w+)\s*\(`)

	// Constructor
	phpConstructorPattern = regexp.MustCompile(`(?m)^\s+(?:(?:public|private|protected)\s+)?function\s+__construct\s*\(`)

	// PHPDoc comment
	phpDocPattern = regexp.MustCompile(`(?s)/\*\*.*?\*/`)

	// Attribute (PHP 8+): #[Route('/path')]
	phpAttributePattern = regexp.MustCompile(`(?m)^\s*#\[[\w\\]+(?:\([^)]*\))?\]`)
)

// phpMatch holds a regex match with metadata.
type phpMatch struct {
	name       string
	symbolType string
	lineNum    int
	matchStart int
	matchEnd   int
}

// Chunk splits PHP source code into function/class-level chunks.
func (p *PHPChunker) Chunk(content []byte, metadata FileMetadata) ([]Chunk, error) {
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	// Extract namespace for context
	namespace := p.extractNamespace(contentStr)

	// Find all symbol declarations
	matches := p.findAllSymbols(contentStr, lines)

	if len(matches) == 0 {
		// Fall back to file-level chunking
		return p.chunkAsFile(content, metadata), nil
	}

	// Sort by line number
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].lineNum < matches[j].lineNum
	})

	// Extract symbols with boundaries
	symbols := p.extractSymbolBoundaries(contentStr, lines, matches, namespace)

	// Merge small symbols if enabled
	if p.config.MergeSmallChunks {
		symbols = p.mergeSmallSymbols(symbols)
	}

	// Convert to chunks
	chunks := make([]Chunk, 0, len(symbols))
	for _, sym := range symbols {
		contentHash := HashContent(sym.content)

		// Include namespace in symbol name for context
		symbolName := sym.name
		if sym.namespace != "" && sym.symbolType == "class" {
			symbolName = sym.namespace + "\\" + sym.name
		}

		chunk := Chunk{
			ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, sym.name, contentHash),
			Content:     sym.content,
			Symbol:      symbolName,
			SymbolType:  sym.symbolType,
			StartLine:   sym.startLine,
			EndLine:     sym.endLine,
			TokenCount:  sym.tokens,
			ContentHash: contentHash,
			FilePath:    metadata.FilePath,
			Language:    "php",
			Module:      sym.namespace,
			ProjectID:   metadata.ProjectID,
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// extractNamespace extracts the namespace from PHP content.
func (p *PHPChunker) extractNamespace(content string) string {
	match := phpNamespacePattern.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

// findAllSymbols finds all symbol declarations in the content.
func (p *PHPChunker) findAllSymbols(content string, lines []string) []phpMatch {
	matches := make([]phpMatch, 0)

	// Helper to add matches from a pattern
	addMatches := func(pattern *regexp.Regexp, symbolType string) {
		for _, loc := range pattern.FindAllStringSubmatchIndex(content, -1) {
			if len(loc) >= 4 {
				name := ""
				if loc[2] >= 0 && loc[3] >= 0 {
					name = content[loc[2]:loc[3]]
				}
				if name == "" {
					// For constructor, use special name
					if symbolType == "constructor" {
						name = "__construct"
					} else {
						continue
					}
				}

				lineNum := strings.Count(content[:loc[0]], "\n") + 1
				matches = append(matches, phpMatch{
					name:       name,
					symbolType: symbolType,
					lineNum:    lineNum,
					matchStart: loc[0],
					matchEnd:   loc[1],
				})
			}
		}
	}

	// Find class-level constructs first (they contain methods)
	addMatches(phpClassPattern, "class")
	addMatches(phpInterfacePattern, "interface")
	addMatches(phpTraitPattern, "trait")
	addMatches(phpEnumPattern, "enum")

	// Then functions (standalone only - methods are inside classes)
	addMatches(phpFunctionPattern, "function")

	// Deduplicate
	return p.deduplicateMatches(matches)
}

// deduplicateMatches removes duplicate matches on the same line.
func (p *PHPChunker) deduplicateMatches(matches []phpMatch) []phpMatch {
	priority := map[string]int{
		"class":       1,
		"interface":   2,
		"trait":       3,
		"enum":        4,
		"function":    5,
		"method":      6,
		"constructor": 7,
	}

	byLine := make(map[int]phpMatch)
	for _, m := range matches {
		if existing, ok := byLine[m.lineNum]; ok {
			if priority[m.symbolType] < priority[existing.symbolType] {
				byLine[m.lineNum] = m
			}
		} else {
			byLine[m.lineNum] = m
		}
	}

	result := make([]phpMatch, 0, len(byLine))
	for _, m := range byLine {
		result = append(result, m)
	}
	return result
}

// extractSymbolBoundaries finds start and end of each symbol using brace matching.
func (p *PHPChunker) extractSymbolBoundaries(content string, lines []string, matches []phpMatch, namespace string) []phpSymbol {
	symbols := make([]phpSymbol, 0, len(matches))

	for i, m := range matches {
		startLine := m.lineNum

		// Look for preceding PHPDoc or attributes
		if startLine > 1 {
			startLine = p.findPrecedingComment(lines, startLine)
		}

		// Find end using brace matching
		endLine := p.findBraceEnd(lines, m.lineNum)

		// Ensure no overlap with next symbol
		if i < len(matches)-1 {
			nextStart := matches[i+1].lineNum
			if endLine >= nextStart {
				endLine = nextStart - 1
			}
		}

		// Bounds check
		if endLine > len(lines) {
			endLine = len(lines)
		}
		if endLine < startLine {
			endLine = startLine
		}

		// Extract content
		symContent := p.extractLines(lines, startLine, endLine)
		tokens := EstimateTokens(symContent)

		symbols = append(symbols, phpSymbol{
			name:       m.name,
			symbolType: m.symbolType,
			startLine:  startLine,
			endLine:    endLine,
			content:    symContent,
			tokens:     tokens,
			namespace:  namespace,
		})
	}

	return symbols
}

// findPrecedingComment finds PHPDoc or attributes before a symbol.
func (p *PHPChunker) findPrecedingComment(lines []string, symbolLine int) int {
	startLine := symbolLine

	for i := symbolLine - 2; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])

		// PHPDoc end
		if strings.HasSuffix(line, "*/") {
			for j := i; j >= 0; j-- {
				if strings.Contains(lines[j], "/**") {
					return j + 1
				}
			}
		}

		// Single-line comment
		if strings.HasPrefix(line, "//") {
			startLine = i + 1
			continue
		}

		// PHP 8 attribute
		if strings.HasPrefix(line, "#[") {
			startLine = i + 1
			continue
		}

		// Empty line - keep looking
		if line == "" {
			continue
		}

		// Other content - stop
		break
	}

	return startLine
}

// findBraceEnd finds the closing brace using brace matching.
func (p *PHPChunker) findBraceEnd(lines []string, startLine int) int {
	depth := 0
	foundOpen := false

	for i := startLine - 1; i < len(lines); i++ {
		line := lines[i]
		inString := false
		stringChar := byte(0)
		inHeredoc := false

		for j := 0; j < len(line); j++ {
			ch := line[j]

			// Simple string handling
			if !inString && !inHeredoc && (ch == '"' || ch == '\'') {
				inString = true
				stringChar = ch
				continue
			}
			if inString && ch == stringChar && (j == 0 || line[j-1] != '\\') {
				inString = false
				continue
			}
			if inString || inHeredoc {
				continue
			}

			// Brace matching
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

	return startLine + 100 // Fallback limit
}

// mergeSmallSymbols merges small symbols into adjacent larger ones.
func (p *PHPChunker) mergeSmallSymbols(symbols []phpSymbol) []phpSymbol {
	if len(symbols) <= 1 {
		return symbols
	}

	result := make([]phpSymbol, 0, len(symbols))
	var pending *phpSymbol

	for i := range symbols {
		sym := symbols[i]

		if sym.tokens < p.config.MinTokens {
			if pending == nil {
				pending = &sym
			} else {
				pending.content += "\n\n" + sym.content
				pending.endLine = sym.endLine
				pending.tokens = EstimateTokens(pending.content)
				pending.name = pending.name + "+" + sym.name
			}
		} else {
			if pending != nil {
				if pending.tokens+sym.tokens <= p.config.MaxTokens {
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

// extractLines extracts lines from startLine to endLine (1-indexed, inclusive).
func (p *PHPChunker) extractLines(lines []string, startLine, endLine int) string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return ""
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

// chunkAsFile creates a single chunk for the entire file.
func (p *PHPChunker) chunkAsFile(content []byte, metadata FileMetadata) []Chunk {
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
		Language:    "php",
		Module:      metadata.Module,
		ProjectID:   metadata.ProjectID,
	}}
}
