# IA_Recuerdo Migración PostgreSQL

## Objetivo

Documentar la transición de IA_Recuerdo hacia PostgreSQL en CT204.

## Estado final

- CT204 hospeda el servicio de memoria
- PostgreSQL corre aparte en la infraestructura local
- El servicio usa PostgreSQL como backend principal
- Los scripts heredados de migración quedaron fuera del flujo productivo

## Notas operativas

- El servicio expone MCP HTTP en `:7438`
- La conexión a base de datos debe apuntar a PostgreSQL real
- Los datos pesados viven en tablas separadas
- El despliegue productivo debe validarse con `healthz` y pruebas de búsqueda

## Verificación

- `GET /healthz`
- `GET /api/v1/stats`
- `GET /api/v1/search`
- `GET /metrics`
