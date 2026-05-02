# Deploy en Proxmox LXC — Problemas y Soluciones

Registro de fallos encontrados durante el primer deploy de `ia-recuerdo` en un LXC Debian 12.
Los comandos usan `pct exec <CT_ID>` — reemplaza `<CT_ID>` con el ID real de tu contenedor Proxmox.

> Documento histórico. El estado actual de CT204 ya está estabilizado y no depende de estos workarounds.

---

## 1. PostgreSQL — Fallo de autenticación (`password authentication failed`)

### Síntoma
```
cannot open store: pgx ping: failed to connect to `user=ia_recuerdo database=ia_recuerdo`:
127.0.0.1:5432 (localhost): failed SASL auth: FATAL: password authentication failed for user "ia_recuerdo"
```

### Causa
El usuario PostgreSQL se creó con `password_encryption = scram-sha-256` (default en PG 15),
pero el intento de autenticación fallaba por un hash mal generado por el escaping multinivel
(`pct exec → bash -c → psql -c "..."`) que mangló la contraseña.

Adicionalmente, `pct exec <CT_ID> -- env PGPASSWORD=... psql` **no pasa variables de entorno**
al interior del container — el proceso ve el env del host, no del CT.

### Solución
Cambiar `pg_hba.conf` a `md5` y recrear el hash en la misma sesión psql:

```bash
# 1. Cambiar pg_hba.conf (líneas host ... scram-sha-256 → md5)
pct exec <CT_ID> -- sed -i 's/scram-sha-256/md5/g' /etc/postgresql/15/main/pg_hba.conf
pct exec <CT_ID> -- pg_ctlcluster 15 main reload

# 2. Crear archivo SQL localmente y copiarlo via pct push (EVITA escaping)
cat > /tmp/pg_alter.sql << 'EOF'
SET password_encryption = md5;
ALTER USER ia_recuerdo WITH PASSWORD '<DB_PASSWORD>';
SELECT usename, passwd FROM pg_shadow WHERE usename='ia_recuerdo';
EOF
pct push <CT_ID> /tmp/pg_alter.sql /tmp/pg_alter.sql
pct exec <CT_ID> -- runuser -u postgres -- psql -f /tmp/pg_alter.sql
```

El resultado correcto en `pg_shadow.passwd` empieza con `md5` (no con `SCRAM-SHA-256$`).

> **Lección**: Para pasar SQL con caracteres especiales a psql dentro de un LXC,
> usar siempre un archivo SQL copiado via `pct push` + `psql -f`, nunca inline con `-c`.

---

## 2. Extensión `vector` no disponible

### Síntoma
```
cannot open store: migration: migration v2_pgvector: v2_pgvector:
ERROR: extension "vector" is not available (SQLSTATE 0A000)
SQL: CREATE EXTENSION IF NOT EXISTS vector
```

### Causa
`postgresql-15-pgvector` no está en los repos base de Debian 12 (`bookworm main`).
El binario lo necesita para las migraciones automáticas.

### Solución
Agregar el repo oficial PGDG e instalar el paquete:

```bash
pct exec <CT_ID> -- bash -c "
  apt-get install -y curl ca-certificates gnupg lsb-release &&
  curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg &&
  echo 'deb [signed-by=/usr/share/keyrings/postgresql.gpg] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main' \
    > /etc/apt/sources.list.d/pgdg.list &&
  apt-get update -qq &&
  apt-get install -y postgresql-15-pgvector
"
```

Luego pre-crear la extensión como superuser (el binario corre como `ia-recuerdo`, sin permisos SUPERUSER):

```bash
pct exec <CT_ID> -- runuser -u postgres -- psql -d ia_recuerdo \
  -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

> **Lección**: `pgvector` requiere instalación explícita via PGDG + pre-creación de la extensión
> con superuser antes del primer arranque del binario. El binario ejecuta la migración
> pero no tiene permisos para crear extensiones.

---

## 3. Servicio systemd sale inmediatamente con `status=0/SUCCESS`

### Síntoma
```
○ ia-recuerdo.service - IA Recuerdo
   Active: inactive (dead) since ... Duration: 9ms
   Process: ExecStart=... (code=exited, status=0/SUCCESS)
```

El servicio arranca, imprime los logs de inicio y termina limpiamente en ~9ms.

### Causa
El flag `-transport both` inicia tanto el servidor HTTP como el servidor MCP/stdio.
En systemd, `stdin` está conectado a `/dev/null` (o similar), lo que causa un **EOF inmediato**
en el reader stdio, haciendo que el proceso termine limpiamente en cuanto arranca.

### Solución
Cambiar `-transport both` a `-transport http` en el service file:

```ini
# /etc/systemd/system/ia-recuerdo.service
ExecStart=/opt/ia-recuerdo/ia-recuerdo \
    -transport http \        ← NO usar "both" en systemd
    -addr :7438 \
    -db-driver postgres \
    -db-dsn ${IA_DB_DSN} \
    -project default
```

> **Lección**: `transport=both` (HTTP + MCP stdio) solo es útil cuando otro proceso mantiene
> el pipe stdin abierto (ej: un agente IA conectado). En systemd, usar `transport=http`.

---

## Resumen de configuración final

| Componente | Valor |
|---|---|
| OS | Debian 12 (bookworm) |
| IP del CT | `<CT_IP>` (asignar IP estática en Proxmox) |
| CT ID | `<CT_ID>` (ej: 204) |
| PostgreSQL | 15.x + pgvector 0.8.2 (PGDG) |
| pg_hba auth | `md5` para 127.0.0.1/32 |
| DB encoding | UTF8 (locale C — recreada desde SQL_ASCII original) |
| Binario | `/opt/ia-recuerdo/ia-recuerdo` (-tags postgres) |
| Transport | `http` para systemd, `both` solo para desarrollo |
| Puerto | `:7438` |
| Healthz | `http://<CT_IP>:7438/healthz` → `{"status":"ok"}` |
| Embed URL | `http://10.0.0.206:11434/v1/embeddings` (Ollama con nomic-embed-text) |
| Embed Model | `nomic-embed-text` (768 dims) |
| Embed Format | `openai` (OpenAI-compatible API) |

---

## Fixes desplegados (2026-05-02)

### Fix 1: URL de Embedding

**Problema:** Embeddings no se generaban para nuevas observaciones.

**Síntoma:** Observaciones guardadas via `mem_save` tenían `embedding IS NULL`.

**Root Cause:** La URL del embedder estaba configurada como `http://10.0.0.206:11434/v1` pero Ollama requiere la ruta completa `/v1/embeddings`.

**Solución:** Actualizar `IA_EMBED_URL` en `/etc/ia-recuerdo/env`:

```bash
IA_EMBED_URL=http://10.0.0.206:11434/v1/embeddings
```

**Verificación:**
```sql
SELECT id, title, embedding IS NOT NULL as has_emb FROM observations ORDER BY id DESC LIMIT 3;
-- id=693,694,695 → has_emb=t (todos con embedding)
```

### Fix 2: Wrapper Script con Flags de Embedding

**Problema:** El wrapper script no pasaba los flags de embedding al binario.

**Solución:** Actualizar `/opt/ia-recuerdo/simple_wrapper.sh`:

```bash
#!/bin/bash
source /etc/ia-recuerdo/env
exec /opt/ia-recuerdo/ia-recuerdo \
  -transport http \
  -addr :7438 \
  -db-driver "$IA_DB_DRIVER" \
  -db-dsn "$IA_DB_DSN" \
  -embed-url "$IA_EMBED_URL" \
  -embed-model "$IA_EMBED_MODEL" \
  -embed-dims "$IA_EMBED_DIMS" \
  -embed-format "$IA_EMBED_FORMAT"
```

### Fix 3: Deployment via pct push

**Nota:** El binario se despliega desde el host Proxmox via `pct push`, no directamente dentro del contenedor.

**Flujo:**
1. Build en Windows: `GOOS=linux GOARCH=amd64 go build`
2. SCP a Proxmox host: `scp binary root@proxmox:/tmp/`
3. Push a CT: `pct push 204 /tmp/binary /opt/ia-recuerdo/ia-recuerdo`
4. Restart: `pct exec 204 -- systemctl restart ia-recuerdo`

---

## Lección aprendida

- Ollama endpoint `/v1/embeddings` (OpenAI-compatible) requiere la ruta **completa**
- El wrapper script debe pasar todos los flags, no solo DSN
- CT204 a veces necesita `systemctl restart` varias veces si el binario está "busy"
- Si `pct push` falla con "Text file busy", esperar o matar el proceso primero
