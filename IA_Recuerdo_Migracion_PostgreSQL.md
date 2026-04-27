# IA_Recuerdo Migración PostgreSQL

## Objetivo

Documentar la transición de IA_Recuerdo hacia PostgreSQL en CT204.

## Estado final

- CT204 hospeda el servicio de memoria
- PostgreSQL corre aparte en la infraestructura local
- El servicio usa PostgreSQL como backend principal
- Los scripts heredados de migración quedaron fuera del flujo productivo
- La conexión productiva de CT204 apunta por IP directa a CT205 (`CT205_IP`), no por DNS

## Notas operativas

- El servicio expone MCP HTTP en `:7438`
- La conexión a base de datos debe apuntar a PostgreSQL real
- Los datos pesados viven en tablas separadas
- El despliegue productivo debe validarse con `healthz` y pruebas de búsqueda
- El servicio systemd debe usar `-transport http` en CT204
- `healthz` y `stats` están operativos sobre el servicio desplegado

## Verificación

- `GET /healthz`
- `GET /api/v1/stats`
- `GET /api/v1/search`
- `GET /metrics`
