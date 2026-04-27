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

## Acceso

- MCP HTTP: `http://<HOST>:7438/mcp`
- REST API: `http://<HOST>:7438/api/v1`

## Estado actual

- PostgreSQL-only en producción
- Búsqueda semántica con `pgvector`
- Adjuntos y relaciones habilitados
- Contenido pesado separado de metadata
- CT204 se conecta a CT205 (PostgreSQL) por IP estática de red interna
- El servicio productivo corre con `-transport http`

## Flujo GitFlow

- `main` para producción
- `develop` para integración
- ramas `feature/*` o `release/*` para cambios
