#!/usr/bin/env bash
# install.sh — Instalación de IA_Recuerdo en Proxmox LXC (Debian/Ubuntu)
#
# Uso (como root en el servidor destino):
#   curl -fsSL https://raw.githubusercontent.com/thiscloud/ia-recuerdo/main/scripts/install.sh | bash
#   # o localmente:
#   bash scripts/install.sh

set -euo pipefail

BIN_SRC="${1:-bin/ia-recuerdo-linux-amd64}"
INSTALL_DIR="/opt/ia-recuerdo"
DATA_DIR="/opt/ia-recuerdo/data"
ENV_FILE="/etc/ia-recuerdo/env"
SERVICE_FILE="/etc/systemd/system/ia-recuerdo.service"

echo "=== IA_Recuerdo Install ==="

# ── 1. Verificar binario ─────────────────────────────────────────
if [ ! -f "$BIN_SRC" ]; then
    echo "ERROR: Binario no encontrado: $BIN_SRC"
    echo "Compila primero con: make build-postgres"
    exit 1
fi

# ── 2. Crear usuario del sistema ─────────────────────────────────
if ! id -u ia-recuerdo &>/dev/null; then
    useradd --system --no-create-home --shell /bin/false ia-recuerdo
    echo "✓ Usuario ia-recuerdo creado"
fi

# ── 3. Directorios ───────────────────────────────────────────────
install -d -o ia-recuerdo -g ia-recuerdo "$INSTALL_DIR" "$DATA_DIR"
echo "✓ Directorios creados"

# ── 4. Binario ───────────────────────────────────────────────────
install -o root -g root -m 0755 "$BIN_SRC" "$INSTALL_DIR/ia-recuerdo"
echo "✓ Binario instalado en $INSTALL_DIR/ia-recuerdo"

# ── 5. Config de entorno ─────────────────────────────────────────
install -d /etc/ia-recuerdo
if [ ! -f "$ENV_FILE" ]; then
    install -o root -g root -m 0640 configs/env.example "$ENV_FILE"
    echo ""
    echo "⚠️  IMPORTANTE: Edita $ENV_FILE con tus credenciales antes de iniciar el servicio"
    echo "   nano $ENV_FILE"
fi

# ── 6. Servicio systemd ──────────────────────────────────────────
install -o root -g root -m 0644 deploy/systemd/ia-recuerdo.service "$SERVICE_FILE"
systemctl daemon-reload
echo "✓ Servicio systemd instalado"

echo ""
echo "=== Instalación completada ==="
echo ""
echo "Próximos pasos:"
echo "  1. Configura la base de datos: nano $ENV_FILE"
echo "  2. Crea la base de datos PostgreSQL:"
echo "       createdb -U postgres ia_recuerdo"
echo "       psql -U postgres -c \"CREATE USER ia_recuerdo WITH PASSWORD 'CHANGE_ME';\""
echo "       psql -U postgres -c \"GRANT ALL ON DATABASE ia_recuerdo TO ia_recuerdo;\""
echo "  3. Inicia el servicio: systemctl enable --now ia-recuerdo"
echo "  4. Verifica: curl http://localhost:7438/healthz"
echo "  5. Crea tu primera API key:"
echo "       # Necesitas un ADMIN_KEY - crea uno en PostgreSQL antes de continuar"
echo "       curl -X POST http://localhost:7438/api/v1/keys -H 'X-Api-Key: ADMIN' -d '{\"name\":\"vscode\"}'"
echo ""
echo "Configuración MCP en VS Code (.vscode/mcp.json):"
echo '  {"servers":{"ia-recuerdo":{"url":"http://localhost:7438/mcp/rpc"}}}'
