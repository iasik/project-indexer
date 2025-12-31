# Project Indexer - RAG Altyapısı

Multi-project, LLM-agnostic, vendor-independent RAG indexleme ve retrieval sistemi.

---

## Genel Bakış

Bu sistem, birden fazla kod projesini indexleyerek LLM/Copilot araçlarının context-aware cevaplar üretmesini sağlar.

### Temel Prensipler

1. **LLM-Agnostic**: Indexleme aşamasında LLM kullanılmaz. Sistem sadece embedding modeli kullanır.
2. **Vendor-Independent**: Embedding provider ve Vector DB değiştirilebilir (config ile).
3. **Multi-Project**: Her proje izole şekilde indexlenir ve sorgulanır.
4. **Deterministic**: Aynı kaynak kod, aynı chunk'ları üretir.
5. **Incremental**: Sadece değişen dosyalar yeniden indexlenir.

---

## Mimari

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            DOCKER COMPOSE                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────────┐   │
│  │   Ollama     │    │    Qdrant    │    │    Retrieval Tool        │   │
│  │  (Embedding) │    │  (Vector DB) │    │    (HTTP API)            │   │
│  │              │    │              │    │                          │   │
│  │ nomic-embed  │    │ Collection:  │    │  POST /retrieve          │   │
│  │ -text        │    │ code_chunks  │    │  GET  /health            │   │
│  │              │    │              │    │                          │   │
│  └──────┬───────┘    └──────┬───────┘    └────────────┬─────────────┘   │
│         │                   │                         │                  │
│         └─────────┬─────────┴─────────────────────────┘                  │
│                   │                                                      │
│         ┌─────────▼─────────┐                                           │
│         │     Indexer       │  (CLI - manuel/cron çalışır)              │
│         │                   │                                           │
│         │ - Full index      │                                           │
│         │ - Incremental     │                                           │
│         │ - Chunk + Embed   │                                           │
│         └───────────────────┘                                           │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         LLM / Copilot (External)                         │
│                                                                          │
│  "Retrieval tool'dan gelen context'e dayanarak cevap ver.               │
│   Context yoksa 'bilmiyorum' de."                                       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Bileşenler

### 1. Indexer (`cmd/indexer/`)

Batch/CLI aracı. Projeleri indexler.

**Sorumluluklar:**
- Proje config'ini oku
- Dosyaları tara (include/exclude patterns)
- Deterministic chunking uygula
- Embedding al (provider üzerinden)
- Vector DB'ye kaydet
- Incremental mod: sadece değişenleri işle

**Çalıştırma:**
```bash
# Full index
docker-compose run indexer --project=crm-backend --full

# Incremental (varsayılan)
docker-compose run indexer --project=crm-backend

# Tüm projeler
docker-compose run indexer --all
```

### 2. Retrieval Tool (`cmd/retrieval-tool/`)

HTTP API server. LLM'lerin çağırdığı tool.

**Sorumluluklar:**
- HTTP endpoint sun (`POST /retrieve`)
- Query'yi embedding'e çevir
- Vector DB'de project_id ile filtreli ara
- Ham context döndür (yorum/özet YOK)
- SIGHUP ile hot reload destekle

**API:**
```
POST /retrieve
GET  /health
```

### 3. Embedding Provider (`internal/embedder/`)

Pluggable embedding katmanı.

**Desteklenen Providerlar:**
| Provider | Model Örneği | Dimensions |
|----------|--------------|------------|
| ollama | nomic-embed-text | 768 |
| openai | text-embedding-3-small | 1536 |
| huggingface | sentence-transformers/* | varies |

### 4. Vector DB Provider (`internal/vectordb/`)

Pluggable vector storage katmanı.

**Desteklenen Providerlar:**
| Provider | Varsayılan Port |
|----------|-----------------|
| qdrant | 6333 |
| milvus | 19530 |
| weaviate | 8080 |

### 5. Chunker (`internal/chunker/`)

Deterministic kod/doküman parçalama.

**Stratejiler:**
| Dosya Tipi | Strateji | Açıklama |
|------------|----------|----------|
| `.go` | function | Go AST ile fonksiyon/struct/interface bazlı |
| `.ts`, `.tsx`, `.js`, `.jsx`, `.vue` | typescript | Regex ile function/class/interface/type/enum/arrow function |
| `.php` | php | Regex ile function/class/method/trait/interface/enum |
| `.md`, `.markdown` | heading | `##`, `###` başlık bazlı |
| diğer | fixed | Token sayısına göre sabit boyut |

**TypeScript/PHP Regex Chunker Özellikleri:**
- JSDoc/PHPDoc yorumları chunk'a dahil
- Brace-depth tracking ile doğru symbol boundary tespiti
- Decorators ve PHP 8 attributes desteği

**Helper Merge Kuralı:**
- `min_chunk_tokens` altındaki fonksiyonlar parent scope'a merge edilir
- Chunk sayısı optimize edilir, context kalitesi korunur

---

## Data Flow

### Indexing Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Source Code │ ──▶ │   Chunker   │ ──▶ │ Chunk-Level │ ──▶ │  Embedder   │
│             │     │             │     │   Diffing   │     │             │
│ .ts, .php   │     │ Chunks +    │     │ Compare     │     │ Only embed  │
│ .go, .md    │     │ Metadata    │     │ hash cache  │     │ changed     │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
       │                   │                   │                   │
       ▼                   ▼                   ▼                   ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Hash Cache  │     │ Chunk Hash  │     │ Delete old  │     │  Vector DB  │
│ (JSON)      │     │ per file    │     │ chunks      │     │  Upsert     │
│ File level  │     │ chunk_id →  │     │ from Qdrant │     │             │
│ + chunk lvl │     │ content_hash│     │             │     │             │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
```

**Chunk-Level Diffing Avantajları:**
- Dosya değiştiğinde sadece değişen chunk'lar re-embed edilir
- Local cache kullanılır (Qdrant'a extra query yok)
- Büyük dosyalarda önemli performans kazancı

### Retrieval Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ LLM Query   │ ──▶ │  Embedder   │ ──▶ │  Vector DB  │ ──▶ │  Response   │
│             │     │             │     │             │     │             │
│ "Login      │     │ Query       │     │ Cosine      │     │ Top-K       │
│  nasıl      │     │ embedding   │     │ similarity  │     │ chunks      │
│  çalışır?"  │     │             │     │ + filter    │     │             │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                                              │
                                              ▼
                                        project_id filter
                                        module filter (optional)
```

---

## Konfigürasyon

### Global Config (`configs/config.yaml`)

```yaml
# =============================================================================
# EMBEDDING PROVIDER
# =============================================================================
embedding:
  # Provider seçimi: ollama | openai | huggingface
  provider: "ollama"
  
  # Model adı (provider'a göre değişir)
  model: "nomic-embed-text"
  
  # Provider endpoint
  endpoint: "http://ollama:11434"
  
  # Embedding boyutu (model'e göre ayarla)
  dimensions: 768
  
  # Batch işleme boyutu
  batch_size: 32
  
  # OpenAI kullanımı için:
  # provider: "openai"
  # model: "text-embedding-3-small"
  # endpoint: "https://api.openai.com/v1"
  # api_key_env: "OPENAI_API_KEY"
  # dimensions: 1536

# =============================================================================
# VECTOR DATABASE PROVIDER
# =============================================================================
vectordb:
  # Provider seçimi: qdrant | milvus | weaviate
  provider: "qdrant"
  
  # Provider endpoint
  endpoint: "http://qdrant:6333"
  
  # Collection adı (tüm projeler tek collection'da, filter ile ayrılır)
  collection_name: "code_chunks"
  
  # Milvus kullanımı için:
  # provider: "milvus"
  # endpoint: "http://milvus:19530"

# =============================================================================
# PROJECT SETTINGS
# =============================================================================
projects:
  # Proje config dosyalarının dizini
  config_dir: "/app/configs/projects"
  
  # Mount edilen kaynak kod base path
  source_base_path: "/sources"

# =============================================================================
# CHUNKING DEFAULTS
# =============================================================================
# Bu değerler proje config'inde override edilebilir
chunking:
  # Minimum chunk token sayısı (altındakiler merge edilir)
  min_tokens: 200
  
  # İdeal chunk boyutu
  ideal_tokens: 500
  
  # Maximum chunk boyutu
  max_tokens: 800
  
  # Küçük chunk'ları parent'a merge et
  merge_small_chunks: true

# =============================================================================
# INDEX CACHE
# =============================================================================
cache:
  # Cache dosyalarının dizini
  dir: "/app/data/index-cache"
  
  # Format: json
  format: "json"

# =============================================================================
# HTTP SERVER (Retrieval Tool)
# =============================================================================
server:
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"

# =============================================================================
# LOGGING
# =============================================================================
logging:
  # Log seviyesi: debug | info | warn | error
  level: "info"
  
  # Format: json | text
  format: "json"
```

### Project Config (`configs/projects/{project_id}.yaml`)

```yaml
# =============================================================================
# PROJECT: CRM Backend
# =============================================================================

# Benzersiz proje tanımlayıcı
project_id: "crm-backend"

# Görüntüleme adı
display_name: "CRM Backend API"

# Kaynak kod yolu (source_base_path'e göre relative)
source_path: "crm-backend"

# =============================================================================
# FILE FILTERS
# =============================================================================

# Dahil edilecek dosya uzantıları
include_extensions:
  - ".go"
  - ".ts"
  - ".tsx"
  - ".php"
  - ".md"
  - ".sql"

# Hariç tutulacak yollar (glob pattern)
exclude_paths:
  - "vendor/"
  - "node_modules/"
  - "testdata/"
  - "*_test.go"
  - ".git/"
  - "*.lock"
  - "*-lock.*"
  - "*.d.ts"

# =============================================================================
# CHUNKING OVERRIDES
# =============================================================================
# Global defaults'ları override eder
chunking:
  code:
    # Kod chunking stratejisi: function | file | fixed
    strategy: "function"
  
  markdown:
    # Markdown chunking stratejisi: heading | paragraph | fixed
    strategy: "heading"
  
  # Bu proje için özel minimum token
  min_tokens: 150

# =============================================================================
# METADATA
# =============================================================================
# Opsiyonel, arama/filtreleme için kullanılabilir
metadata:
  team: "backend"
  tags:
    - "api"
    - "golang"
    - "auth"
```

---

## Provider Ekleme Rehberi

### Yeni Embedding Provider Eklemek

1. `internal/embedder/` altında yeni dosya oluştur:

```go
// internal/embedder/newprovider.go

type NewProviderEmbedder struct {
    client *http.Client
    config Config
}

func (e *NewProviderEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
    // Implementation
}

func (e *NewProviderEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
    // Implementation
}

func (e *NewProviderEmbedder) ModelInfo() ModelInfo {
    return ModelInfo{
        Name:       e.config.Model,
        Dimensions: e.config.Dimensions,
    }
}
```

2. Factory'ye register et:

```go
// internal/embedder/factory.go

func NewProvider(cfg Config) (Provider, error) {
    switch cfg.Provider {
    case "ollama":
        return NewOllamaEmbedder(cfg)
    case "openai":
        return NewOpenAIEmbedder(cfg)
    case "newprovider":  // ← Ekle
        return NewNewProviderEmbedder(cfg)
    default:
        return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
    }
}
```

3. Config'de kullan:

```yaml
embedding:
  provider: "newprovider"
  model: "model-name"
  endpoint: "http://newprovider:8000"
```

### Yeni Vector DB Provider Eklemek

Aynı pattern: interface implement et, factory'ye register et.

---

## Chunk Metadata Yapısı

Her chunk aşağıdaki metadata ile Vector DB'ye kaydedilir:

```json
{
  "id": "crm-backend:auth/service.go:Login:abc123",
  "vector": [0.123, -0.456, ...],
  "payload": {
    "project_id": "crm-backend",
    "file_path": "auth/service.go",
    "symbol": "Login",
    "symbol_type": "function",
    "language": "go",
    "module": "auth",
    "start_line": 45,
    "end_line": 78,
    "content": "func Login(ctx context.Context, ...) { ... }",
    "content_hash": "sha256:abc123...",
    "indexed_at": "2025-12-31T10:30:00Z"
  }
}
```

---

## Cache Yapısı

Her proje için `data/index-cache/{project_id}.json` dosyasında cache tutulur:

```json
{
  "project_id": "bee-flora",
  "updated_at": "2025-12-31T01:07:37Z",
  "files": {
    "src/components/Button.tsx": {
      "content_hash": "abc123...",
      "mod_time": "2025-12-31T01:00:00Z",
      "indexed_at": "2025-12-31T01:07:00Z",
      "chunk_ids": ["bee-flora:src/components/Button.tsx:Button:def456"],
      "chunk_hashes": {
        "bee-flora:src/components/Button.tsx:Button:def456": "sha256:def456..."
      }
    }
  }
}
```

**Chunk-Level Diffing:**
- `chunk_hashes` sayesinde dosya değiştiğinde sadece değişen chunk'lar re-embed edilir
- Yeni chunk → embed + upsert
- Değişen chunk → re-embed + upsert
- Silinen chunk → Qdrant'tan delete

---

## Oversized Chunks Raporu

Token limitini aşan chunk'lar `data/index-cache/reports/{project_id}-oversized.json` dosyasına kaydedilir:

```json
{
  "project_id": "bee-flora",
  "generated_at": "2025-12-31T01:07:37Z",
  "total_count": 10,
  "max_tokens_allowed": 2048,
  "chunks": [
    {
      "file_path": "src/views/appointment/index.tsx",
      "symbol": "AppointmentView",
      "token_count": 2540,
      "max_allowed": 2048,
      "content_size_bytes": 6351
    }
  ]
}
```

---

## HTTP API

### POST /retrieve

Semantic search ile ilgili kod parçalarını döner.

**Request:**
```json
{
  "project_id": "crm-backend",
  "query": "Login sırasında token nasıl yenileniyor?",
  "top_k": 5,
  "filters": {
    "module": "auth",
    "language": "go"
  }
}
```

**Response:**
```json
{
  "results": [
    {
      "content": "func RefreshToken(ctx context.Context, oldToken string) (*Token, error) {\n    // Validate old token\n    claims, err := validateToken(oldToken)\n    ...\n}",
      "source": "auth/token.go",
      "symbol": "RefreshToken",
      "project_id": "crm-backend",
      "score": 0.89
    },
    {
      "content": "...",
      "source": "auth/service.go",
      "symbol": "Login",
      "project_id": "crm-backend",
      "score": 0.82
    }
  ],
  "query_time_ms": 45
}
```

### GET /health

**Response:**
```json
{
  "status": "healthy",
  "components": {
    "vectordb": "ok",
    "embedder": "ok"
  },
  "version": "1.0.0"
}
```

### Error Responses

Tüm hatalar standart bir format ile döner:

```json
{
  "error": "project_id is required",
  "code": "MISSING_REQUIRED_FIELD",
  "request_id": "a1b2c3d4e5f6g7h8"
}
```

**Error Codes:**
| Code | HTTP Status | Açıklama |
|------|-------------|----------|
| `INVALID_REQUEST` | 400 | Geçersiz JSON body |
| `MISSING_REQUIRED_FIELD` | 400 | Zorunlu alan eksik |
| `EMBEDDING_FAILED` | 500 | Embedding oluşturulamadı |
| `SEARCH_FAILED` | 500 | Vector DB sorgusu başarısız |
| `SERVICE_DEGRADED` | 503 | Provider bağlantısı sorunlu |
```

---

## Hot Reload

Retrieval Tool, SIGHUP sinyali ile config'i yeniden yükler.

```bash
# Config değiştirdikten sonra
docker kill -s HUP project-indexer-retrieval-tool-1

# Veya container içinden
kill -HUP 1
```

**Reload edilen ayarlar:**
- Proje listesi (yeni proje eklendi/silindi)
- Provider endpoint'leri
- Chunking defaults
- Log level

**Reload EDİLMEYEN ayarlar (restart gerekir):**
- HTTP port
- Provider değişikliği (ollama → openai)

---

## Docker Compose Kullanımı

**Servisleri başlat:**
```bash
docker-compose up -d
```

**Model indir (ilk seferlik):**
```bash
docker-compose exec ollama ollama pull nomic-embed-text
```

**Proje indexle:**
```bash
SOURCES_PATH=/path/to/projects docker-compose run indexer --project=crm-backend
```

**API test:**
```bash
curl -X POST http://localhost:8080/retrieve \
  -H "Content-Type: application/json" \
  -d '{"project_id": "crm-backend", "query": "auth flow", "top_k": 5}'
```

---

## LLM Tool Tanımı (OpenAI Function Calling Format)

```json
{
  "name": "retrieve_code_context",
  "description": "Retrieves relevant code snippets and documentation from the indexed codebase. Use this tool to find implementation details, function definitions, and related code before answering questions about the codebase.",
  "parameters": {
    "type": "object",
    "properties": {
      "project_id": {
        "type": "string",
        "description": "The project identifier to search within (e.g., 'crm-backend', 'randevu-app')"
      },
      "query": {
        "type": "string",
        "description": "Natural language query describing what code or information you're looking for"
      },
      "top_k": {
        "type": "integer",
        "description": "Number of results to return (default: 5, max: 20)",
        "default": 5
      },
      "filters": {
        "type": "object",
        "properties": {
          "module": {
            "type": "string",
            "description": "Filter by module/package name"
          },
          "language": {
            "type": "string",
            "description": "Filter by programming language"
          }
        }
      }
    },
    "required": ["project_id", "query"]
  }
}
```

---

## Edge Cases & Performans Notları

### Edge Cases

| Durum | Davranış |
|-------|----------|
| Boş dosya | Skip edilir, indexlenmez |
| Binary dosya | Skip edilir (extension filter) |
| Çok büyük dosya (>1MB) | Uyarı loglanır, max_tokens ile chunklara bölünür |
| Oversized chunk (>2048 token) | Embedding alınır (truncate), rapor dosyasına eklenir |
| UTF-8 olmayan dosya | Skip edilir, hata loglanır |
| Proje config bulunamadı | Hata döner, indexleme durmaz |
| Vector DB bağlantı hatası | Retry (3x), sonra fail |
| Embedding API hatası | Retry (3x), sonra chunk skip |

### Performans Optimizasyonları

1. **Batch Embedding**: Chunk'lar batch_size'a göre gruplandırılarak embedding alınır
2. **Parallel File Processing**: Dosyalar goroutine'ler ile paralel işlenir
3. **Chunk-Level Diffing**: Dosya değiştiğinde sadece değişen chunk'lar re-embed edilir
4. **Local Hash Cache**: Chunk hash'leri local cache'de saklanır (Qdrant'a extra query yok)
5. **Connection Pooling**: HTTP client'lar connection reuse yapar
6. **Vector DB Bulk Upsert**: Chunk'lar tek seferde toplu eklenir
7. **Progress Reporting**: ETA hesaplamalı batch-level ilerleme gösterimi
8. **Oversized Detection**: Token limitini aşan chunk'lar JSON rapora kaydedilir

### Önerilen Limitler

| Parametre | Önerilen Değer | Açıklama |
|-----------|----------------|----------|
| batch_size | 32 | Embedding batch boyutu |
| max_tokens | 800 | Chunk başına max token |
| top_k | 5-10 | Retrieval sonuç sayısı |
| Proje dosya sayısı | <10,000 | Full index ~5-10dk |
| Collection boyutu | <1M vectors | Qdrant performanslı kalır |

---

## Hızlı Başlangıç

```bash
# 1. Repo'yu clone et
git clone <repo-url>
cd project-indexer

# 2. Örnek config'leri kopyala
cp configs/config.yaml.example configs/config.yaml

# 3. Proje config'i oluştur
vim configs/projects/myproject.yaml

# 4. Servisleri başlat
docker-compose up -d

# 5. Embedding modelini indir
docker-compose exec ollama ollama pull nomic-embed-text

# 6. Projeyi indexle
SOURCES_PATH=/path/to/your/code docker-compose run indexer --project=myproject

# 7. Test et
curl -X POST http://localhost:8080/retrieve \
  -H "Content-Type: application/json" \
  -d '{"project_id": "myproject", "query": "main function", "top_k": 3}'
```

---

## Lisans

MIT
