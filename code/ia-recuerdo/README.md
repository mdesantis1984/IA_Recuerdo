# IA_Recuerdo

Sistema de memoria persistente centralizado para agentes IA.

## Resumen

- MCP por `stdio` y `HTTP`
- REST API para observaciones, sesiones, prompts, exportación y métricas
- PostgreSQL como persistencia principal
- `pgvector` para búsqueda semántica
- Filtros por `tags`
- Consolidación de proyectos
- Adjuntos y relaciones entre observaciones
- Contenido pesado separado de metadata

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

## Modelo de datos

- `observations`: metadata ligera
- `observation_content`: contenido completo
- `attachments`: binarios por observación
- `observation_relations`: vínculos explícitos
- `sessions`: historial de sesiones
- `prompts`: prompts reutilizables
- `api_keys`: claves de acceso REST
