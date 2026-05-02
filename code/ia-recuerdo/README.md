# IA_Recuerdo

**Versión actual:** `1.1.0`

## Propósito
`IA_Recuerdo` es el MCP de memoria persistente centralizada para agentes IA. Su objetivo es almacenar, buscar y relacionar observaciones, decisiones, configuraciones y aprendizazgos de forma eficiente y trazable desde OpenCode y Visual Studio Code.

## Objetivos
- Proveer una sola capa de acceso a memoria persistent.
- Permitir captura de contexto sin salir del flujo del editor.
- Mantener trazabilidad de decisiones y descubrimientos.
- Unificar memoria de sesión, proyecto y personal.
- Exponer observaciones citables, resumidas y reutilizables.
- Soportar deduplicación semántica automática.

## Alcance
Incluye:
- memoria persistente por proyecto y sesión
- búsqueda full-text y semántica (embeddings)
- relaciones entre observaciones
- adjuntos ybinarios por observación
- topics y tags灵活的
- MCP por `stdio` y `HTTP`
- REST API para observaciones, sesiones, prompts, exportación y métricas
- PostgreSQL como persistencia principal con `pgvector`
- smart upsert con deduplicación semántica
- historial y caché de contexto
- API keys con scopes
- telemetría operativa

No incluye:
- edición masiva de observaciones externas
- publicación automática
- UI autónoma fuera de MCP
- entrenamiento de modelos
- autenticación OAuth externa

## Requisitos funcionales
- **FR-01**: Guardar observaciones con title, content, type, project, scope, tags.
- **FR-02**: Actualizar observaciones existentes por ID.
- **FR-03**: Eliminar observaciones (soft-delete por defecto, hard-delete opcional).
- **FR-04**: Sugerir topic_key estable antes de guardar.
- **FR-05**: Buscar observaciones por query full-text.
- **FR-06**: Buscar observaciones por similitud semántica usando embeddings.
- **FR-07**: Obtener contexto reciente de sesiones previas.
- **FR-08**: Obtener timeline cronológico alrededor de una observación.
- **FR-09**: Guardar prompts reutilizables.
- **FR-10**: Iniciar y cerrar sesiones de trabajo.
- **FR-11**: Registrar resúmenes de sesión con accomplishments y discoveries.
- **FR-12**: Extraer aprendizajes pasivos de texto.
- **FR-13**: Fusionar proyectos con nombres variantes.
- **FR-14**: Guardar y listar adjuntos binarios.
- **FR-15**: Guardar y listar relaciones entre observaciones.
- **FR-16**: Exponer métricas del sistema.
- **FR-17**: Exportar e importar observaciones.
- **FR-18**: API keys con scopes (read, write, admin).

## Requisitos no funcionales
- **NFR-01**: Baja latencia para operaciones comunes.
- **NFR-02**: Respuesta estructurada y predecible.
- **NFR-03**: Alta mantenibilidad por capas.
- **NFR-04**: Trazabilidad de fuentes y decisiones.
- **NFR-05**: Compatibilidad con varios clientes MCP.
- **NFR-06**: Configuración externa sin hardcode.
- **NFR-07**: Observabilidad básica obligatoria.
- **NFR-08**: Degradación parcial aceptable si embedding no está disponible.
- **NFR-09**: Seguridad por defecto (API keys, scopes).
- **NFR-10**: Escalabilidad horizontal futura sin rediseño completo.

## Criterios de usuario
- El agente puede guardar información y recuperarla sin salir del editor.
- El usuario ve claridad en qué proyecto y sesión se almacenó.
- La respuesta tiene poco ruido y mucho valor práctico.
- El sistema no pierde información cuando no tiene conexión a embedding.
- Las observaciones similares se fusionan automáticamente cuando corresponde.

## Arquitectura lógica

### 1. Ingress / API MCP
- Valida inputs de tools.
- Normaliza consultas.
- Decide caché o ejecución en vivo.
- Publica tools, resources y prompts.

### 2. Store layer
- PostgreSQL con pgvector para persistencia.
- Workers async para embeddings post-save.
- Smart upsert con deduplicación semántica.
- Separa metadata de contenido pesado.

### 3. Embedding provider
- HTTP provider para Ollama o OpenAI.
- Genera vectores de 768 dims (nomic-embed-text).
- No bloquea el save principal (async goroutine).

### 4. Cache layer
- Mantiene entradas normalizadas de contexto.
- Invalida después de escritura.

### 5. Observability layer
- Métricas via `/metrics`.
- Logs estructurados.
- Health check via `/healthz`.

## Modelo de datos

### Observation
- `id`: identificador único
- `title`: título corto (5-10 palabras)
- `content`: contenido completo
- `type`: decision|bugfix|pattern|config|discovery|learning|architecture
- `project`: nombre del proyecto
- `scope`: project|personal
- `topic_key`: clave de upsert (generada automáticamente si no se provee)
- `tags`: array de tags opcionales
- `session_id`: referencia a sesión
- `embedding`: vector de 768 dims (para búsqueda semántica)
- `duplicate_count`: número de fusiones realizadas
- `revision_count`: número de actualizaciones
- `created_at`, `updated_at`, `last_seen_at`: timestamps

### Session
- `id`: session UUID
- `project`: proyecto
- `agent`: nombre del agente
- `goal`: objetivo de la sesión
- `started_at`, `ended_at`: timestamps

### Prompt
- `id`: identificador
- `content`: contenido del prompt
- `project`: proyecto
- `created_at`: timestamp

### APIKey
- `id`: UUID
- `name`: nombre identificador
- `key_hash`: hash de la clave
- `scopes`: read,write,admin
- `created_at`, `expires_at`, `revoked`: metadatos

## Catálogo final de tools

### Memoria
- `mem_save`: Guardar observación estructurada
- `mem_update`: Actualizar observación por ID
- `mem_delete`: Eliminar observación (soft o hard)
- `mem_suggest_topic_key`: Sugerir topic_key antes de guardar

### Búsqueda
- `mem_search`: Búsqueda full-text
- `mem_semantic_search`: Búsqueda por similitud semántica
- `mem_context`: Contexto reciente de sesión/proyecto
- `mem_timeline`: Timeline cronológico
- `mem_get_observation`: Contenido completo de observación

### Sesiones
- `mem_session_start`: Iniciar sesión
- `mem_session_end`: Cerrar sesión
- `mem_session_summary`: Guardar resumen de sesión
- `mem_capture_passive`: Extraer aprendizajes de texto

### Gestión
- `mem_save_prompt`: Guardar prompt reutilizable
- `mem_stats`: Métricas del sistema
- `mem_merge_projects`: Fusionar proyectos

### Relaciones
- `mem_save_attachment`: Guardar adjunto binario
- `mem_list_attachments`: Listar adjuntos
- `mem_save_relation`: Guardar relación entre observaciones
- `mem_list_relations`: Listar relaciones

## Reglas por tool
- Una tool no modifica lo que no le pertenece.
- Una tool devuelve solo lo que el agente necesita por defecto.
- Las tools de búsqueda deben citar fuentes.
- Las tools de eliminación deben confirmar el tipo (soft/hard).
- Las tools de relación no deben crear ciclos.

## Criterios de aceptación
- Cada tool definida tiene propósito, entrada, salida y error esperable.
- El embedding se genera async post-save sin bloquear.
- La caché invalida después de escritura.
- La salida es compatible con OpenCode y VS Code.
- El sistema tolera degradación parcial si embedding no está disponible.
- La seguridad de entrada y salida está cubierta.
- La observabilidad está definida.

## Orden de implementación sugerido
1. Núcleo MCP y validación de inputs.
2. Store layer con PostgreSQL y pgvector.
3. Tools básicas de memoria (save, update, delete, search).
4. Embedding provider y búsqueda semántica.
5. Sesiones y timeline.
6. Adjuntos y relaciones.
7. Smart upsert con deduplicación.
8. Caché y control.
9. Recursos y prompts.
10. Observabilidad y seguridad reforzada.
11. Tests y hardening.

## Entregables documentales del repo final
- README técnico
- contrato MCP
- catálogo de tools
- ejemplos de uso para OpenCode y VS Code
- guía de configuración
- ADR-001 para smart upsert
- guía de observabilidad
- guía de seguridad
- guía de troubleshooting

## Criterio de cierre
`IA_Recuerdo` solo se considera listo cuando:
- todo el catálogo está definido
- el contrato MCP está estable
- las dependencias están documentadas
- las decisiones de transporte y seguridad están cerradas
- la estructura de repo está lista para codificar
- smart upsert y embedding generation funcionan

## Seguridad común

- La autenticación por API-key es opcional.
- Una misma API-key puede reutilizarse en varios MCPs.
- El acceso sin API-key debe seguir siendo válido por defecto.
- Si se habilita autenticación, aceptar `X-Api-Key` y `Authorization: Bearer`.
- Los permisos deben modelarse por `scope`.
- Recomendación de scopes: `read`, `write`, `admin`.

## Modelo exacto de scopes

- `owner`: acceso máximo del MCP, solo recomendado para uso privado.
- `read`: lectura y consulta.
- `write`: escritura de observaciones.
- `admin`: configuración y mantenimiento.

Scopes del MCP:
- `read`
- `write`
- `admin`
- `owner`

## Contrato de auth

- El token es una API-key opaca validada del lado del servidor.
- `X-Api-Key` y `Authorization: Bearer` son equivalentes.
- Si ambos llegan, `X-Api-Key` tiene prioridad.
- `403 insufficient_scope` debe usarse cuando la credencial es válida pero no alcanza.

## Configuración de ambiente

```bash
# Base de datos
IA_DB_DRIVER=postgres
IA_DB_DSN=postgres://user:pass@<DB_HOST>:5432/db?sslmode=disable

# Transport
IA_TRANSPORT=http|stdio|both
IA_ADDR=:7438

# Proyecto
IA_PROJECT=default

# Embeddings (Ollama)
IA_EMBED_URL=http://<OLLAMA_HOST>:11434/v1/embeddings
IA_EMBED_MODEL=nomic-embed-text
IA_EMBED_DIMS=768
IA_EMBED_FORMAT=openai

# Smart Upsert
IA_UPSERT_ENABLED=true
IA_UPSERT_THRESHOLD_UPDATE=0.85
IA_UPSERT_THRESHOLD_RELATED=0.75
IA_UPSERT_WORKERS=2

# Caché (opcional, Valkey)
IA_VALKEY=<VALKEY_HOST>:6379
```

## Deployment

Ver: [DEPLOY-CT204-FIXES.md](deploy/DEPLOY-CT204-FIXES.md)

Ver: [ADR-001_Smart_Upsert_Semantic_Dedupe.md](../ADR-001_Smart_Upsert_Semantic_Dedupe.md)
