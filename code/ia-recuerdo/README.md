# IA_Recuerdo

> CT204: memoria persistente centralizada para agentes IA.

## Qué es

IA_Recuerdo es el servicio de memoria de la infraestructura IA local. Expone MCP y REST API sobre PostgreSQL, con soporte para búsqueda semántica, adjuntos y relaciones entre observaciones.

## Dónde vive

- Código: [`code/ia-recuerdo`](code/ia-recuerdo)
- SDD: [`IA_Recuerdo.md`](IA_Recuerdo.md)
- Migración/operación: [`IA_Recuerdo_Migracion_PostgreSQL.md`](IA_Recuerdo_Migracion_PostgreSQL.md)

## Infraestructura

- CT203: orquestador MCP
- CT204: servicio de memoria persistente
- CT205: PostgreSQL 15 + pgvector
- CT206: Ollama para embeddings (`nomic-embed-text`)

## Acceso

- MCP HTTP: `http://<HOST>:7438/mcp`
- REST API: `http://<HOST>:7438/api/v1`

## Características

| Característica | Descripción |
|---|---|
| Memoria persistente | observations con metadata y contenido separado |
| Búsqueda semántica | pgvector con embeddings de 768 dims |
| Smart Upsert | Deduplicación basada en similitud semántica (ADR-001) |
| MCP tools | 18 tools para memoria, búsqueda, sesiones y gestión |
| API keys | Autenticación con scopes configurables |
| Async workers | Generación de embeddings sin bloquear el save |

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

## Estado actual

- PostgreSQL-only en producción
- Búsqueda semántica con `pgvector` funcionando
- Smart Upsert ADR-001 habilitado con workers async
- Embeddings generados post-INSERT (async)
- Adjuntos y relaciones habilitados
- Contenido pesado separado de metadata
- CT204 se conecta a CT205 (PostgreSQL) por IP estática de red interna
- El servicio productivo corre con `-transport http`
- Embed URL: `http://<OLLAMA_HOST>:11434/v1/embeddings`

## Changelog

### 1.1.0 — 2026-05-02
- Fix: URL de embedding corregida `/v1` → `/v1/embeddings`
- Feature: Embedding generation en Store layer (async post-INSERT)
- Feature: Smart upsert ADR-001 con deduplicación semántica
- Feature: ADR-001 documentado y tests implementados
- Refactor: README restructurado

### 1.0.0 — 2026-04-26
- Servicio MCP de memoria inicial
- 18 tools MCP registradas
- PostgreSQL con pgvector para persistencia
- API keys con scopes
- REST API completa

## Flujo GitFlow

- `main` para producción
- `develop` para integración
- ramas `feature/*` o `release/*` para cambios

## Seguridad

- API keys con hashbcrypt para autenticación
- Scopes configurables (read, write, admin, owner)
- Sin telemetría ni envío de datos a terceros
- Validación de inputs en todos los endpoints

## Licencia

MIT © ThisCloud Services
