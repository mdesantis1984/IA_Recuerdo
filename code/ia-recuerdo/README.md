# IA_Recuerdo

> Sistema de memoria persistente centralizado — potenciación de [Engram](https://github.com/Gentleman-Programming/engram) accesible sin binario local.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://golang.org)
[![MCP](https://img.shields.io/badge/MCP-2024--11--05-blueviolet)](https://spec.modelcontextprotocol.io/)
[![SQLite](https://img.shields.io/badge/SQLite-embedded-blue?logo=sqlite)](https://pkg.go.dev/modernc.org/sqlite)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-pgvector-336791?logo=postgresql)](https://github.com/pgvector/pgvector)

## Diferencias clave vs Engram

| Característica | Engram | IA_Recuerdo |
|---|---|---|
| Transporte | Solo stdio (local) | **stdio + HTTP MCP** (remoto) |
| Base de datos | SQLite | SQLite (dev) / **PostgreSQL** (prod) |
| Búsqueda semántica | No | **pgvector** (Ollama/OpenAI) |
| Caché | No | **Valkey** |
| Instalación en cliente | Binario obligatorio | **No requerida** |
| Puerto | 7437 | **7438** |

## Inicio rápido

```bash
# Desarrollo (SQLite, HTTP)
make run

# Producción (PostgreSQL)
make build-postgres
./ia-recuerdo -transport both -addr :7438 -db-driver postgres -db-dsn "postgres://..."
```

## Configuración MCP en VS Code

Sin instalar nada en el cliente:

```json
{
  "servers": {
    "ia-recuerdo": {
      "url": "http://<HOST>:7438/mcp/rpc"
    }
  }
}
```

## Configuración MCP drop-in Engram (stdio)

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

## 15 MCP Tools disponibles

Compatibles con Engram — los agentes migran transparentemente:

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
# Salud
curl http://localhost:7438/healthz

# Crear API key
curl -X POST http://localhost:7438/api/v1/keys \
  -H "X-Api-Key: ADMIN_KEY" \
  -d '{"name":"vscode-agent"}'

# Buscar
curl "http://localhost:7438/api/v1/search?q=postgres" \
  -H "X-Api-Key: ir_xxx"

# Exportar (migración desde Engram)
curl http://localhost:7438/api/v1/export -H "X-Api-Key: ir_xxx" > backup.json

# Importar desde Engram
curl -X POST http://localhost:7438/api/v1/import \
  -H "X-Api-Key: ir_xxx" \
  -H "Content-Type: application/json" \
  --data-binary @engram-export.json
```

## Migración desde Engram

```bash
export IA_RECUERDO_KEY=ir_xxxxx
bash scripts/migrate-from-engram.sh
```

## Búsqueda semántica (pgvector)

Requiere: PostgreSQL con la extensión `pgvector` instalada + proveedor de embeddings.

### Ollama (self-hosted, recomendado)

```bash
# 1. Instalar Ollama y descargar modelo
curl -fsSL https://ollama.ai/install.sh | sh
ollama pull nomic-embed-text  # 274 MB

# 2. Instalar pgvector en Postgres (Debian/Ubuntu)
apt install postgresql-16-pgvector

# 3. Arrancar IA_Recuerdo con embeddings
./ia-recuerdo \
  -db-driver postgres -db-dsn "postgres://..." \
  -embed-url http://localhost:11434/v1/embeddings \
  -embed-model nomic-embed-text \
  -embed-dims 768
```

### OpenAI

```bash
./ia-recuerdo \
  -db-driver postgres -db-dsn "postgres://..." \
  -embed-url https://api.openai.com/v1/embeddings \
  -embed-model text-embedding-3-small \
  -embed-token sk-xxx \
  -embed-dims 1536
```

### Usando la herramienta

```json
{"name": "mem_semantic_search", "arguments": {"query": "error de autenticación en JWT", "project": "mi-proyecto"}}
```

Si no hay embedder configurado, `mem_semantic_search` no queda habilitado para uso semántico.

## Instalación en Proxmox

```bash
make build-postgres
scp bin/ia-recuerdo-linux-amd64-postgres root@<HOST>:/tmp/ia-recuerdo-linux-amd64
bash scripts/install.sh
```

## Makefile targets

```
make build          # SQLite, dev
make build-linux    # Linux amd64, SQLite
make build-postgres # Linux amd64, PostgreSQL (producción)
make run            # HTTP :7438 con SQLite
make run-both       # HTTP + stdio
make test           # Tests
make docker-build   # Imagen Docker
make deploy-k8s     # Kubernetes
make health         # curl /healthz
make export         # Exportar todas las observaciones
make import         # Importar desde JSON
```

---

## Registrar IA_Recuerdo MCP en clientes IA

El servidor MCP de **IA_Recuerdo** se puede conectar desde cualquier agente IA: VS Code, Visual Studio 2022/2026, Claude Code, Cursor, etc. **No requiere autenticación** en el endpoint `/mcp`.

### Crear una API Key (solo para REST API)

El endpoint MCP no necesita autenticación. La API key solo es necesaria para usar la REST API (`/api/v1/*`):

```bash
./ia-recuerdo -create-token "mi-cliente"
# → ir_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

### VS Code y Visual Studio — archivo de configuración único

Tanto **VS Code** como **Visual Studio 2022/2026** con GitHub Copilot leen el archivo:

```
%USERPROFILE%\.mcp.json
```

Añade la entrada `ia-recuerdo` junto a cualquier otro servidor que ya tengas:

```json
{
  "inputs": [],
  "servers": {
    "ia-recuerdo": {
      "type": "http",
      "url": "http://<HOST>:7438/mcp"
    }
  }
}
```

Reinicia el cliente para cargar la nueva configuración.

---

### Verificar conectividad

```bash
# MCP (sin auth)
curl -s -X POST http://<HOST>:7438/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}'

# REST API (requiere X-Api-Key)
curl http://<HOST>:7438/api/v1/search?q=test \
  -H "X-Api-Key: ir_xxx"
```
```

**Ver documentación detallada:** [INSTALAR_MCP_CLIENTE.md](INSTALAR_MCP_CLIENTE.md)

---

## Notas

- CT203 es el orquestador MCP.
- CT204 es el servicio de memoria persistente.
- La documentación de transición Engram se conserva solo como legado.

