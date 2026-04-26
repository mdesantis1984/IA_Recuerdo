#!/usr/bin/env bash
# LEGACY: migrate-from-engram.sh — Migra TODO el historial de Engram a IA_Recuerdo
# Conservado solo como referencia histórica de transición.
#
# Prerequisitos:
#   - engram corriendo en :7437
#   - ia-recuerdo corriendo en :7438
#   - IA_RECUERDO_KEY configurado (API key de ia-recuerdo)
#
# Uso:
#   export IA_RECUERDO_KEY=ir_xxxxx
#   bash scripts/migrate-from-engram.sh
#
# Post-migración:
#   1. Verificar conteo: curl http://localhost:7438/healthz
#   2. Apagar engram: systemctl stop engram
#   3. Actualizar mcp.json de los agentes para apuntar a ia-recuerdo

set -euo pipefail

ENGRAM_ADDR="${ENGRAM_ADDR:-http://localhost:7437}"
IA_ADDR="${IA_ADDR:-http://localhost:7438}"
IA_KEY="${IA_RECUERDO_KEY:?IA_RECUERDO_KEY must be set}"
EXPORT_FILE="${TMPDIR:-/tmp}/engram-export-$(date +%Y%m%d-%H%M%S).json"

echo "=== LEGACY Migración Engram → IA_Recuerdo ==="
echo "Engram: $ENGRAM_ADDR"
echo "IA_Recuerdo: $IA_ADDR"
echo ""

# ── 1. Exportar desde Engram ─────────────────────────────────────
echo "[1/4] Exportando observaciones de Engram..."
# Engram v1.11 expone /api/observations para export
HTTP_STATUS=$(curl -s -o "$EXPORT_FILE" -w "%{http_code}" \
    "$ENGRAM_ADDR/api/observations")

if [ "$HTTP_STATUS" != "200" ]; then
    echo "ERROR: Engram export falló con HTTP $HTTP_STATUS"
    echo "Intenta con: curl $ENGRAM_ADDR/api/observations"
    exit 1
fi

COUNT=$(python3 -c "import json,sys; d=json.load(open('$EXPORT_FILE')); print(len(d) if isinstance(d,list) else d.get('count',0))" 2>/dev/null || echo "?")
echo "   Exportadas $COUNT observaciones → $EXPORT_FILE"

# ── 2. Adaptar formato si necesario ──────────────────────────────
echo "[2/4] Adaptando formato Engram → IA_Recuerdo..."
ADAPTED_FILE="${EXPORT_FILE%.json}-adapted.json"
python3 - <<'PYEOF' "$EXPORT_FILE" "$ADAPTED_FILE"
import json, sys

src, dst = sys.argv[1], sys.argv[2]
raw = json.load(open(src))

# Engram puede devolver lista directa o {observations:[...]}
if isinstance(raw, list):
    obs_list = raw
elif isinstance(raw, dict):
    obs_list = raw.get("observations", raw.get("data", []))
else:
    obs_list = []

adapted = []
for o in obs_list:
    adapted.append({
        "title":           o.get("title", "Imported observation"),
        "content":         o.get("content", o.get("observation", "")),
        "type":            o.get("type", o.get("observation_type", "discovery")),
        "project":         o.get("project", "default"),
        "scope":           o.get("scope", "project"),
        "topic_key":       o.get("topic_key") or o.get("topicKey"),
        "tags":            o.get("tags", []),
        "session_id":      o.get("session_id") or o.get("sessionId"),
        "duplicate_count": o.get("duplicate_count", 0),
        "revision_count":  o.get("revision_count", 0),
        "created_at":      o.get("created_at") or o.get("createdAt") or "2024-01-01T00:00:00Z",
        "updated_at":      o.get("updated_at") or o.get("updatedAt") or "2024-01-01T00:00:00Z",
        "last_seen_at":    o.get("last_seen_at") or o.get("lastSeenAt") or "2024-01-01T00:00:00Z",
    })

result = {"observations": adapted}
with open(dst, "w") as f:
    json.dump(result, f, indent=2, default=str)

print(f"   Adaptadas {len(adapted)} observaciones")
PYEOF

# ── 3. Importar en IA_Recuerdo ───────────────────────────────────
echo "[3/4] Importando en IA_Recuerdo..."
IMPORT_RESULT=$(curl -s -X POST \
    -H "X-Api-Key: $IA_KEY" \
    -H "Content-Type: application/json" \
    --data-binary "@$ADAPTED_FILE" \
    "$IA_ADDR/api/v1/import")

echo "   $IMPORT_RESULT"

# ── 4. Verificar ─────────────────────────────────────────────────
echo "[4/4] Verificando..."
HEALTH=$(curl -s "$IA_ADDR/healthz")
echo "   IA_Recuerdo health: $HEALTH"

echo ""
echo "=== Migración completada ==="
echo ""
echo "Próximos pasos:"
echo "  1. Verificar datos: curl -H 'X-Api-Key: $IA_KEY' $IA_ADDR/api/v1/stats | jq"
echo "  2. Apagar Engram: systemctl stop engram"
echo "  3. Actualizar mcp.json de agentes:"
echo '     {"name":"ia-recuerdo","url":"'"$IA_ADDR"'/mcp/rpc"}'
echo ""
echo "Los archivos de export se quedaron en:"
echo "  Original:  $EXPORT_FILE"
echo "  Adaptado:  $ADAPTED_FILE"
