# IA_Recuerdo SDD

## Objetivo

IA_Recuerdo es un servidor MCP de memoria persistente para agentes IA, con API REST, búsqueda semántica y soporte para adjuntos y relaciones.

## Componentes

- MCP HTTP y `stdio`
- REST API
- Persistencia PostgreSQL
- `pgvector` para búsqueda semántica
- Tabla separada para contenido largo
- Adjuntos binarios
- Relaciones entre observaciones

## Modelo

- `observations`: metadata, títulos y preview corto
- `observation_content`: contenido completo
- `attachments`: binarios asociados
- `observation_relations`: vínculos entre observaciones
- `sessions`, `prompts`, `api_keys`: soporte operativo

## Alcance funcional

- Guardar, editar, buscar y eliminar observaciones
- Buscar por texto y por semántica
- Consolidar proyectos duplicados
- Guardar adjuntos y relaciones
- Exponer métricas básicas

## Despliegue

- Producción en CT204
- Acceso por HTTP en `:7438`
- PostgreSQL como backend único en producción
- CT204 se conecta a CT205 por IP directa (`10.0.0.205`)
- El servicio systemd corre con `-transport http`
- `GET /healthz` y `GET /api/v1/stats` deben responder para considerar el despliegue sano
