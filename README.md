# IA_Recuerdo

> CT204: memoria persistente centralizada para agentes IA, accesible sin binario local.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://golang.org)
[![MCP](https://img.shields.io/badge/MCP-2024--11--05-blueviolet)](https://spec.modelcontextprotocol.io/)
[![SQLite](https://img.shields.io/badge/SQLite-dev-blue?logo=sqlite)](https://pkg.go.dev/modernc.org/sqlite)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-pgvector-336791?logo=postgresql)](https://github.com/pgvector/pgvector)

## Baseline CT204

| Característica | IA_Recuerdo |
|---|---|
| Transporte | `stdio + HTTP MCP` |
| Base de datos | SQLite (dev) / PostgreSQL (prod) |
| Búsqueda semántica | pgvector (Ollama/OpenAI) |
| Caché | No usada |
| Instalación en cliente | No requerida |
| Puerto | 7438 |

## Inicio rápido

```bash
# Desarrollo (SQLite, HTTP)
make run

# Producción (PostgreSQL)
make build-postgres
./ia-recuerdo -transport both -addr :7438 -db-driver postgres -db-dsn "postgres://..."
```

## Configuración MCP en VS Code

```json
{
  "servers": {
    "ia-recuerdo": {
      "url": "http://<HOST>:7438/mcp"
    }
  }
}
```

## Configuración MCP drop-in

```json
{
  "servers": {
    "ia-recuerdo": {
      "command": "ia-recuerdo",
      "args": ["-transport", "stdio"]
    }
  }
}
```

## 16 MCP Tools disponibles

Contrato actual de `internal/mcp/tools.go`:

| Tool | Descripción |
|---|---|
| `mem_save` | Guardar observación (upsert por topic_key) |
| `mem_update` | Actualizar por ID |
| `mem_delete` | Soft/hard delete |
| `mem_suggest_topic_key` | Generar key estable para upserts |
| `mem_search` | Búsqueda full-text |
| `mem_context` | Contexto reciente de sesión |
| `mem_timeline` | Contexto temporal alrededor de una observación |
| `mem_get_observation` | Observación completa por ID |
| `mem_session_start` | Registrar inicio de sesión |
| `mem_session_end` | Marcar sesión como completa |
| `mem_session_summary` | Guardar resumen de sesión |
| `mem_save_prompt` | Guardar prompt reutilizable |
| `mem_stats` | Estadísticas del sistema |
| `mem_capture_passive` | Extraer aprendizajes de texto |
| `mem_merge_projects` | Fusionar nombres de proyecto |
| `mem_semantic_search` | Búsqueda semántica por embeddings |

## REST API

```bash
curl http://localhost:7438/healthz
curl -X POST http://localhost:7438/api/v1/keys \
  -H "X-Api-Key: ADMIN_KEY" \
  -d '{"name":"vscode-agent"}'
```

## Migración heredada

Los scripts `scripts/migrate-from-engram.sh` y `scripts/migrate-engram.py` quedan como legado de transición.

## Notas

- CT204 escucha en `:7438`.
- La documentación de transición Engram se conserva solo como legado.
