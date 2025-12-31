package chunker

import (
	"strings"
	"testing"
)

func TestTypeScriptChunker_Functions(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`
// Helper function
function add(a: number, b: number): number {
    return a + b;
}

/**
 * Multiplies two numbers
 */
export function multiply(x: number, y: number): number {
    return x * y;
}

export async function fetchData(url: string): Promise<Response> {
    const response = await fetch(url);
    return response;
}
`)

	metadata := FileMetadata{
		FilePath:  "math.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	// Check that we found the named functions
	foundAdd := false
	foundMultiply := false
	foundFetchData := false

	for _, c := range chunks {
		if strings.Contains(c.Symbol, "add") {
			foundAdd = true
			if c.SymbolType != "function" {
				t.Errorf("Expected 'add' to be type 'function', got %s", c.SymbolType)
			}
		}
		if strings.Contains(c.Symbol, "multiply") {
			foundMultiply = true
			// Should capture JSDoc comment
			if !strings.Contains(c.Content, "Multiplies two numbers") {
				t.Error("Expected 'multiply' chunk to contain JSDoc comment")
			}
		}
		if strings.Contains(c.Symbol, "fetchData") {
			foundFetchData = true
		}
	}

	if !foundAdd {
		t.Error("Expected to find 'add' function")
	}
	if !foundMultiply {
		t.Error("Expected to find 'multiply' function")
	}
	if !foundFetchData {
		t.Error("Expected to find 'fetchData' function")
	}
}

func TestTypeScriptChunker_ClassesAndInterfaces(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        800,
		MergeSmallChunks: false,
	})

	content := []byte(`
interface User {
    id: number;
    name: string;
    email: string;
}

export interface Product extends BaseModel {
    title: string;
    price: number;
}

class UserService {
    private users: User[] = [];

    public getUser(id: number): User | undefined {
        return this.users.find(u => u.id === id);
    }

    public addUser(user: User): void {
        this.users.push(user);
    }
}

export abstract class BaseController {
    protected logger: Logger;

    constructor(logger: Logger) {
        this.logger = logger;
    }
}
`)

	metadata := FileMetadata{
		FilePath:  "models.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundInterface := false
	foundClass := false

	for _, c := range chunks {
		if c.SymbolType == "interface" {
			foundInterface = true
		}
		if c.SymbolType == "class" {
			foundClass = true
		}
	}

	if !foundInterface {
		t.Error("Expected to find at least one interface")
	}
	if !foundClass {
		t.Error("Expected to find at least one class")
	}
}

func TestTypeScriptChunker_TypesAndEnums(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        20,
		IdealTokens:      100,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`
type Status = 'pending' | 'active' | 'completed';

export type UserRole = 'admin' | 'user' | 'guest';

enum Color {
    Red = 'red',
    Green = 'green',
    Blue = 'blue',
}

export const enum Direction {
    Up,
    Down,
    Left,
    Right,
}
`)

	metadata := FileMetadata{
		FilePath:  "types.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundType := false
	foundEnum := false

	for _, c := range chunks {
		if c.SymbolType == "type" {
			foundType = true
		}
		if c.SymbolType == "enum" {
			foundEnum = true
		}
	}

	if !foundType {
		t.Error("Expected to find at least one type alias")
	}
	if !foundEnum {
		t.Error("Expected to find at least one enum")
	}
}

func TestTypeScriptChunker_ArrowFunctions(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        20,
		IdealTokens:      100,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`
const greet = (name: string): string => {
    return "Hello, " + name;
};

export const calculate = (x: number, y: number) => x + y;

const asyncHandler = async (req: Request) => {
    const data = await fetchData(req.url);
    return data;
};
`)

	metadata := FileMetadata{
		FilePath:  "utils.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundArrow := false
	for _, c := range chunks {
		if c.SymbolType == "arrow_function" {
			foundArrow = true
			break
		}
	}

	if !foundArrow {
		t.Error("Expected to find at least one arrow function")
	}
}

func TestTypeScriptChunker_ReactComponents(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        800,
		MergeSmallChunks: false,
	})

	content := []byte(`
import React from 'react';

interface ButtonProps {
    label: string;
    onClick: () => void;
}

export function Button({ label, onClick }: ButtonProps) {
    return (
        <button onClick={onClick}>
            {label}
        </button>
    );
}

const Card: React.FC<{ title: string }> = ({ title, children }) => {
    return (
        <div className="card">
            <h2>{title}</h2>
            {children}
        </div>
    );
};

export default Card;
`)

	metadata := FileMetadata{
		FilePath:  "components.tsx",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk for React components")
	}

	// Check that Button is found
	foundButton := false
	for _, c := range chunks {
		if strings.Contains(c.Symbol, "Button") {
			foundButton = true
			break
		}
	}

	if !foundButton {
		t.Error("Expected to find Button component")
	}
}

func TestTypeScriptChunker_DeterministicOutput(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`
function foo(): void {
    console.log("foo");
}

function bar(): void {
    console.log("bar");
}
`)

	metadata := FileMetadata{
		FilePath:  "test.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	// Run twice and compare
	chunks1, _ := chunker.Chunk(content, metadata)
	chunks2, _ := chunker.Chunk(content, metadata)

	if len(chunks1) != len(chunks2) {
		t.Fatalf("Determinism failed: different chunk counts %d vs %d", len(chunks1), len(chunks2))
	}

	for i := range chunks1 {
		if chunks1[i].ID != chunks2[i].ID {
			t.Errorf("Determinism failed: chunk %d has different IDs", i)
		}
		if chunks1[i].ContentHash != chunks2[i].ContentHash {
			t.Errorf("Determinism failed: chunk %d has different content hashes", i)
		}
	}
}

func TestTypeScriptChunker_EmptyFile(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(``)

	metadata := FileMetadata{
		FilePath:  "empty.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed on empty file: %v", err)
	}

	// Empty file should produce a single file-level chunk or empty
	if len(chunks) > 1 {
		t.Errorf("Expected 0 or 1 chunk for empty file, got %d", len(chunks))
	}
}

func TestTypeScriptChunker_NoSymbols(t *testing.T) {
	chunker := NewTypeScriptChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`
// Just some comments
// No actual code symbols

const x = 1;
const y = 2;
`)

	metadata := FileMetadata{
		FilePath:  "constants.ts",
		Language:  "typescript",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	// Should fall back to file-level chunking
	if len(chunks) == 0 {
		t.Error("Expected at least one chunk for file fallback")
	}
}
