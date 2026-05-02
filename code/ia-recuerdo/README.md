# IA_Recuerdo

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![MCP](https://img.shields.io/badge/MCP-Compatible-FF6B6B?logo=robot)](https://modelcontextprotocol.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Servicio MCP de memoria persistente centralizada para agentes IA locales. Usa PostgreSQL + pgvector para búsqueda semántica y CT206 para embeddings.

---

## Resumen

- Memoria persistente por proyecto y sesión.
- Búsqueda full-text y semántica (embeddings).
- Smart upsert con deduplicación semántica automática.
- 18 tools MCP registradas.
- Relaciones y adjuntos entre observaciones.
- API keys con scopes (read, write, admin).

---

## Características principales

| Característica | Descripción |
|---|---|
| Memoria persistente | observations con metadata y contenido separado |
| Búsqueda semántica | pgvector con embeddings de 768 dims |
| Smart Upsert | Deduplicación basada en similitud semántica |
| MCP tools | 18 tools para memoria, búsqueda, sesiones y gestión |
| REST API | Endpoints para observaciones, búsqueda, métricas |
| API keys | Autenticación con scopes configurables |
| Async workers | Generación de embeddings sin bloquear el save |

---

## Tools MCP (18 disponibles)

| Tool | Descripción |
|---|---|
| `mem_save` | Guardar observación estructurada |
| `mem_update` | Actualizar observación por ID |
| `mem_delete` | Eliminar observación (soft o hard) |
| `mem_suggest_topic_key` | Sugerir topic_key antes de guardar |
| `mem_search` | Búsqueda full-text |
| `mem_semantic_search` | Búsqueda por similitud semántica |
| `mem_context` | Contexto reciente de sesión/proyecto |
| `mem_timeline` | Timeline cronológico |
| `mem_get_observation` | Contenido completo de observación |
| `mem_session_start` | Iniciar sesión |
| `mem_session_end` | Cerrar sesión |
| `mem_session_summary` | Guardar resumen de sesión |
| `mem_capture_passive` | Extraer aprendizajes de texto |
| `mem_save_prompt` | Guardar prompt reutilizable |
| `mem_stats` | Métricas del sistema |
| `mem_merge_projects` | Fusionar proyectos |
| `mem_save_attachment` | Guardar adjunto binario |
| `mem_list_relations` | Listar relaciones entre observaciones |

---

## Acceso

| Endpoint | URL | Descripción |
|---|---|---|
| MCP HTTP | `http://<HOST>:7438/mcp` | Protocolo MCP para agentes IA |
| Health | `http://<HOST>:7438/healthz` | Verificación de estado del servicio |

---

## Requisitos

1. CT204:7438 ejecutándose como servicio Go.
2. PostgreSQL 15+ con extensión pgvector.
3. CT206:11434 con Ollama y modelo `nomic-embed-text`.
4. API keys configuradas para acceso REST.

---

## Uso rápido (MCP)

```json
{
  "method": "tools/call",
  "params": {
    "name": "mem_save",
    "arguments": {
      "title": "Decisión de arquitectura",
      "content": "Usar PostgreSQL con pgvector para embeddings",
      "type": "decision",
      "project": "mi-proyecto"
    }
  }
}
```

---

## Configuración (ejemplo)

```jsonc
{
  "mcp_server": {
    "host": "<HOST>",
    "port": 7438,
    "transport": "http"
  },
  "database": {
    "driver": "postgres",
    "dsn": "postgres://user:pass@<DB_HOST>:5432/ia_recuerdo?sslmode=disable"
  },
  "embeddings": {
    "url": "http://<OLLAMA_HOST>:11434/v1/embeddings",
    "model": "nomic-embed-text",
    "dims": 768
  },
  "smart_upsert": {
    "enabled": true,
    "threshold_update": 0.85,
    "threshold_related": 0.75,
    "workers": 2
  }
}
```

---

## Arquitectura

```
CT204 (Go Service :7438)
  │
  ├─ MCP Handler
  │   └─ 18 Tools registradas
  │
  ├─ Store Layer
  │   ├─ PostgreSQL + pgvector
  │   ├─ Async embedding workers
  │   └─ Smart upsert con deduplicación
  │
  ├─ Embedding Provider
  │   └─ CT206 Ollama :11434
  │       └─ nomic-embed-text (768 dims)
  │
  ├─ Cache Layer
  │   └─ Valkey (opcional)
  │
  └─ REST API
      ├─ /api/v1/observations
      ├─ /api/v1/search
      ├─ /api/v1/stats
      └─ /api/v1/keys
```

---

## Changelog

### 1.1.0 — 2026-05-02
- Fix: URL de embedding corregida `/v1` → `/v1/embeddings`.
- Feature: Embedding generation en Store layer (async post-INSERT).
- Feature: Smart upsert ADR-001 con deduplicación semántica.
- Feature: ADR-001 documentado y tests implementados.
- Refactor: README match IA_Buscar structure.

### 1.0.0 — 2026-04-26
- Servicio MCP de memoria inicial.
- 18 tools MCP registradas.
- PostgreSQL con pgvector para persistencia.
- API keys con scopes.
- REST API completa.

---

## Seguridad

- API keys con hashbcrypt para autenticación.
- Scopes configurables (read, write, admin, owner).
- Sin telemetría ni envío de datos a terceros.
- Validación de inputs en todos los endpoints.

---

## Licencia

MIT © ThisCloud Services
