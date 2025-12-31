package chunker

import (
	"strings"
	"testing"
)

func TestPHPChunker_Functions(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

/**
 * Add two numbers
 */
function add($a, $b) {
    return $a + $b;
}

function multiply($x, $y) {
    return $x * $y;
}
`)

	metadata := FileMetadata{
		FilePath:  "math.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	foundAdd := false
	foundMultiply := false

	for _, c := range chunks {
		if strings.Contains(c.Symbol, "add") {
			foundAdd = true
			if c.SymbolType != "function" {
				t.Errorf("Expected 'add' to be type 'function', got %s", c.SymbolType)
			}
			// Should capture PHPDoc comment
			if !strings.Contains(c.Content, "Add two numbers") {
				t.Error("Expected 'add' chunk to contain PHPDoc comment")
			}
		}
		if strings.Contains(c.Symbol, "multiply") {
			foundMultiply = true
		}
	}

	if !foundAdd {
		t.Error("Expected to find 'add' function")
	}
	if !foundMultiply {
		t.Error("Expected to find 'multiply' function")
	}
}

func TestPHPChunker_Classes(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        1000,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

namespace App\Models;

/**
 * User model
 */
class User extends Model
{
    protected $fillable = ['name', 'email'];

    public function posts()
    {
        return $this->hasMany(Post::class);
    }

    public function profile()
    {
        return $this->hasOne(Profile::class);
    }
}
`)

	metadata := FileMetadata{
		FilePath:  "User.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundClass := false
	foundNamespace := false

	for _, c := range chunks {
		if c.SymbolType == "class" {
			foundClass = true
			// Check namespace is captured in module or symbol
			if c.Module == "App\\Models" {
				foundNamespace = true
			}
		}
	}

	if !foundClass {
		t.Error("Expected to find User class")
	}
	if !foundNamespace {
		t.Error("Expected namespace to be captured")
	}
}

func TestPHPChunker_Interfaces(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        30,
		IdealTokens:      100,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

namespace App\Contracts;

interface UserRepositoryInterface
{
    public function find(int $id): ?User;
    public function create(array $data): User;
    public function update(int $id, array $data): User;
    public function delete(int $id): bool;
}
`)

	metadata := FileMetadata{
		FilePath:  "UserRepositoryInterface.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundInterface := false
	for _, c := range chunks {
		if c.SymbolType == "interface" {
			foundInterface = true
			break
		}
	}

	if !foundInterface {
		t.Error("Expected to find interface")
	}
}

func TestPHPChunker_Traits(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        30,
		IdealTokens:      100,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

namespace App\Traits;

trait HasTimestamps
{
    public function touchTimestamp(): void
    {
        $this->updated_at = now();
    }

    public function getCreatedAtAttribute(): Carbon
    {
        return Carbon::parse($this->attributes['created_at']);
    }
}
`)

	metadata := FileMetadata{
		FilePath:  "HasTimestamps.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundTrait := false
	for _, c := range chunks {
		if c.SymbolType == "trait" {
			foundTrait = true
			break
		}
	}

	if !foundTrait {
		t.Error("Expected to find trait")
	}
}

func TestPHPChunker_Enums(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        30,
		IdealTokens:      100,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

namespace App\Enums;

enum Status: string
{
    case Pending = 'pending';
    case Active = 'active';
    case Completed = 'completed';
    case Cancelled = 'cancelled';
}
`)

	metadata := FileMetadata{
		FilePath:  "Status.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	foundEnum := false
	for _, c := range chunks {
		if c.SymbolType == "enum" {
			foundEnum = true
			break
		}
	}

	if !foundEnum {
		t.Error("Expected to find enum")
	}
}

func TestPHPChunker_LaravelController(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        1000,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

namespace App\Http\Controllers;

use App\Models\User;
use Illuminate\Http\Request;

class UserController extends Controller
{
    /**
     * Display a listing of users.
     */
    public function index()
    {
        $users = User::all();
        return view('users.index', compact('users'));
    }

    /**
     * Store a newly created user.
     */
    public function store(Request $request)
    {
        $validated = $request->validate([
            'name' => 'required|string|max:255',
            'email' => 'required|email|unique:users',
        ]);

        $user = User::create($validated);
        return redirect()->route('users.show', $user);
    }
}
`)

	metadata := FileMetadata{
		FilePath:  "UserController.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk for Laravel controller")
	}

	// Check that UserController class is found
	foundController := false
	for _, c := range chunks {
		if strings.Contains(c.Symbol, "UserController") {
			foundController = true
			break
		}
	}

	if !foundController {
		t.Error("Expected to find UserController class")
	}
}

func TestPHPChunker_PHP8Attributes(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

namespace App\Http\Controllers;

use App\Attributes\Route;

class ApiController
{
    #[Route('/api/users', methods: ['GET'])]
    public function listUsers()
    {
        return response()->json(User::all());
    }

    #[Route('/api/users/{id}', methods: ['GET'])]
    public function getUser(int $id)
    {
        return response()->json(User::find($id));
    }
}
`)

	metadata := FileMetadata{
		FilePath:  "ApiController.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	// PHP 8 attributes should be included in the chunk content
	foundAttribute := false
	for _, c := range chunks {
		if strings.Contains(c.Content, "#[Route") {
			foundAttribute = true
			break
		}
	}

	if !foundAttribute {
		t.Error("Expected PHP 8 attributes to be captured in chunk content")
	}
}

func TestPHPChunker_DeterministicOutput(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php

function foo() {
    echo "foo";
}

function bar() {
    echo "bar";
}
`)

	metadata := FileMetadata{
		FilePath:  "test.php",
		Language:  "php",
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

func TestPHPChunker_EmptyFile(t *testing.T) {
	chunker := NewPHPChunker(ChunkingConfig{
		MinTokens:        50,
		IdealTokens:      200,
		MaxTokens:        500,
		MergeSmallChunks: false,
	})

	content := []byte(`<?php
`)

	metadata := FileMetadata{
		FilePath:  "empty.php",
		Language:  "php",
		ProjectID: "test-project",
	}

	chunks, err := chunker.Chunk(content, metadata)
	if err != nil {
		t.Fatalf("Chunk failed on empty file: %v", err)
	}

	// Empty/minimal file should produce a single file-level chunk
	if len(chunks) > 1 {
		t.Errorf("Expected 0 or 1 chunk for empty file, got %d", len(chunks))
	}
}
