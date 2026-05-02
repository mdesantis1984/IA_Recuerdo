# IA_Recuerdo

Sistema de memoria persistente centralizado para agentes IA.

## Capacidades

- MCP por `stdio` y `HTTP`
- REST API para observaciones, sesiones, prompts, exportación y métricas
- PostgreSQL como persistencia principal
- `pgvector` para búsqueda semántica
- Filtros por `tags`
- Consolidación de proyectos
- Adjuntos y relaciones entre observaciones
- Contenido pesado separado de metadata

## Inicio rápido

```bash
make build-postgres
./ia-recuerdo -transport both -addr :7438 -db-driver postgres -db-dsn "postgres://..."
```

## Configuración MCP

```json
{
  "servers": {
    "ia-recuerdo": {
      "url": "http://<HOST>:7438/mcp"
    }
  }
}
```

## MCP tools

- `mem_save`
- `mem_update`
- `mem_delete`
- `mem_suggest_topic_key`
- `mem_search`
- `mem_context`
- `mem_timeline`
- `mem_get_observation`
- `mem_session_start`
- `mem_session_end`
- `mem_session_summary`
- `mem_save_prompt`
- `mem_stats`
- `mem_capture_passive`
- `mem_merge_projects`
- `mem_semantic_search`
- `mem_save_attachment`
- `mem_list_attachments`
- `mem_save_relation`
- `mem_list_relations`

## REST API

- `GET /healthz`
- `GET /api/v1/observations`
- `POST /api/v1/observations`
- `GET /api/v1/observations/{id}`
- `DELETE /api/v1/observations/{id}`
- `GET /api/v1/context`
- `GET /api/v1/search`
- `GET /api/v1/stats`
- `GET /api/v1/export`
- `POST /api/v1/import`
- `GET /api/v1/keys`
- `POST /api/v1/keys`
- `DELETE /api/v1/keys/{id}`
- `GET /metrics`

## Configuración de Embeddings

Para búsqueda semántica, el sistema usa un servicio Ollama dedicado (CT206).

```bash
# Endpoint del servicio de embeddings
OLLAMA_EMBEDDINGS_URL=http://10.0.0.206:11434/v1/embeddings
OLLAMA_EMBEDDINGS_MODEL=nomic-embed-text
```

> **Importante:** La URL debe incluir `/v1/embeddings` completo. Ollama requiere la ruta exacta.

## Smart Upsert (Deduplicación Semántica)

El sistema soporta deduplicación automática basada en similitud semántica:

```bash
IA_UPSERT_ENABLED=true
IA_UPSERT_THRESHOLD_UPDATE=0.85
IA_UPSERT_THRESHOLD_RELATED=0.75
IA_UPSERT_WORKERS=2
```

Cuando `topic_key` no se especifica, el sistema genera uno automático basado en el tipo y título.

Ver: [ADR-001_Smart_Upsert_Semantic_Dedupe.md](../ADR-001_Smart_Upsert_Semantic_Dedupe.md)

## Modelo de datos

- `observations`: metadata ligera
- `observation_content`: contenido completo
- `attachments`: binarios por observación
- `observation_relations`: vínculos explícitos
- `sessions`: historial de sesiones
- `prompts`: prompts reutilizables
- `api_keys`: claves de acceso REST
