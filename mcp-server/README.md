# Project Indexer MCP Server

VS Code Copilot entegrasyonu için MCP (Model Context Protocol) server.

## Kurulum

```bash
cd mcp-server
npm install
npm run build
```

## VS Code Ayarları

`.vscode/settings.json` veya global settings'e ekleyin:

```json
{
  "github.copilot.chat.codeGeneration.useInstructionFiles": true,
  "mcp": {
    "servers": {
      "project-indexer": {
        "command": "node",
        "args": ["/Users/iasik/projects/project-indexer/mcp-server/dist/index.js"],
        "env": {
          "RETRIEVAL_API_URL": "http://localhost:8080"
        }
      }
    }
  }
}
```

## Kullanım

MCP server etkinleştirildiğinde Copilot otomatik olarak şu tool'ları kullanabilir:

### `search_codebase`

Indexed codebase'de semantic arama yapar:

```
@workspace Login fonksiyonu nasıl çalışıyor?
```

Copilot otomatik olarak `search_codebase` tool'unu çağırır ve sonuçları context'e ekler.

### `list_projects`

Index'lenmiş projeleri listeler.

## Örnek Kullanımlar

1. **Kod Arama:**
   ```
   Kullanıcı yetkilendirme işlemleri nerede yapılıyor?
   ```

2. **Belirli Projede Arama:**
   ```
   bee-flora projesinde randevu oluşturma kodu
   ```

3. **Pattern Arama:**
   ```
   API error handling pattern'leri
   ```

## Gereksinimler

- Retrieval API'nin çalışıyor olması (`docker compose up -d`)
- Node.js 18+
- VS Code 1.96+ (MCP desteği için)

## Troubleshooting

### MCP Server Görünmüyor

1. VS Code'u yeniden başlatın
2. `Cmd+Shift+P` → "MCP: List Servers" ile kontrol edin
3. Logs: `Cmd+Shift+P` → "MCP: Show Logs"

### API'ye Bağlanamıyor

```bash
# API'nin çalıştığını kontrol edin
curl http://localhost:8080/health

# Docker container'ları kontrol edin
docker compose ps
```
