# Project Indexer

Multi-project, LLM-agnostic RAG indexleme ve retrieval sistemi.

## Özellikler

- 🔌 **Vendor-Independent**: Embedding ve Vector DB provider'ları config ile değiştirilebilir
- 📁 **Multi-Project**: Birden fazla projeyi izole şekilde indexle ve sorgula
- ⚡ **Incremental**: Sadece değişen dosyaları yeniden indexle
- 🔄 **Hot Reload**: SIGHUP ile config değişikliklerini uygula
- 🐳 **Docker-Ready**: `docker-compose up` ile hemen kullanıma hazır

## Hızlı Başlangıç

```bash
# 1. Config'i oluştur
cp configs/config.yaml.example configs/config.yaml

# 2. Proje config'i ekle
cp configs/projects/example-project.yaml configs/projects/myproject.yaml
# Düzenle: project_id, source_path, include_extensions

# 3. Servisleri başlat
docker-compose up -d

# 4. Embedding modelini indir (ilk seferlik)
docker-compose exec ollama ollama pull nomic-embed-text

# 5. Projeyi indexle
SOURCES_PATH=/path/to/code docker-compose run indexer --project=myproject

# 6. API'yi test et
curl -X POST http://localhost:8080/retrieve \
  -H "Content-Type: application/json" \
  -d '{"project_id": "myproject", "query": "main function", "top_k": 5}'
```

## Mimari

Detaylı mimari için: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Ollama    │     │   Qdrant    │     │  Retrieval  │
│  (Embed)    │     │ (Vector DB) │     │    Tool     │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └─────────┬─────────┴───────────────────┘
                 │
       ┌─────────▼─────────┐
       │     Indexer       │
       └───────────────────┘
```

## Komutlar

```bash
# Full index
docker-compose run indexer --project=myproject --full

# Incremental index (varsayılan)
docker-compose run indexer --project=myproject

# Tüm projeleri indexle
docker-compose run indexer --all

# Config hot reload
docker kill -s HUP project-indexer-retrieval-tool-1
```

## API

### POST /retrieve

```json
{
  "project_id": "myproject",
  "query": "authentication flow",
  "top_k": 5,
  "filters": {
    "module": "auth"
  }
}
```

### GET /health

```json
{
  "status": "healthy",
  "components": {
    "vectordb": "ok",
    "embedder": "ok"
  }
}
```

## Konfigürasyon

- `configs/config.yaml` - Global sistem ayarları
- `configs/projects/*.yaml` - Proje bazlı ayarlar

Detaylı config referansı için [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#konfigürasyon) bölümüne bakın.

## Lisans

MIT
