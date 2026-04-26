# IA_Recuerdo

> CT204: memoria persistente centralizada para agentes IA.

## Puntos clave

- Código principal: `code/ia-recuerdo`
- Despliegue productivo: PostgreSQL + MCP HTTP
- Documentación SDD: `IA_Recuerdo.md`
- Migración y operación: `IA_Recuerdo_Migracion_PostgreSQL.md`

## Infraestructura

- CT203: orquestador MCP
- CT204: servicio de memoria persistente

## Acceso

- MCP HTTP en `http://<HOST>:7438/mcp`
- REST API en `http://<HOST>:7438/api/v1`

## Estado

- PostgreSQL-only en producción
- Búsqueda semántica con `pgvector`
- Adjuntos y relaciones habilitados
- Contenido pesado separado de metadata
