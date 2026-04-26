# IA_Recuerdo

Sistema de memoria persistente centralizado para agentes IA.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://golang.org)
[![MCP](https://img.shields.io/badge/MCP-2024--11--05-blueviolet)](https://spec.modelcontextprotocol.io/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-pgvector-336791?logo=postgresql)](https://github.com/pgvector/pgvector)

## Resumen

- Transporte MCP por `stdio` y `HTTP`.
- REST API para observaciones, sesiones, prompts, exportación y métricas.
- PostgreSQL como persistencia única.
- `pgvector` para búsqueda semántica.
- Caché operativa para contexto y búsquedas.
- Filtros por `tags` en búsqueda y contexto.
- Consolidación de proyectos duplicados.
- Métricas HTTP básicas en `/metrics`.
- Adjuntos/binarios por observación.
- Relaciones explícitas entre observaciones.
- Contenido pesado separado de metadata.

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
      "type": "http",
      "url": "http://<HOST>:7438/mcp"
    }
  }
}
```

## Herramientas MCP

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
- `mem_save_attachment`
- `mem_list_attachments`
- `mem_save_relation`
- `mem_list_relations`
- `mem_semantic_search`

## Modelo de datos

- `observations`: metadata ligera y preview de texto.
- `observation_content`: contenido textual completo separado.
- `attachments`: binarios asociados a observaciones.
- `observation_relations`: vínculos entre observaciones.

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

- `observations`: metadata ligera de cada memoria.
- `observation_content`: contenido largo separado de la metadata.
- `attachments`: binarios y adjuntos por observación.
- `observation_relations`: vínculos explícitos entre observaciones.
- `sessions`: historial de sesiones de agente.
- `prompts`: prompts reutilizables.
- `api_keys`: claves opacas para acceso REST.

## Mejoras recientes

- Búsqueda y contexto con filtro opcional por `tags`.
- `mem_merge_projects` para consolidar variantes de proyecto.
- `mem_save_attachment` y `mem_list_attachments`.
- `mem_save_relation` y `mem_list_relations`.
- `mem_semantic_search` con fallback a búsqueda textual.
- `/metrics` para observabilidad mínima.
- Separación de contenido pesado en tabla dedicada.

## Búsqueda semántica

Requiere PostgreSQL con `pgvector` habilitado y un provider de embeddings.

Si no hay provider de embeddings, `mem_semantic_search` degrada a `mem_search` automáticamente.

## Documentación

- `IA_Recuerdo.md` en la raíz del workspace contiene el SDD completo.
