# Registrar IA_Recuerdo MCP en clientes IA

**IA_Recuerdo** expone un servidor MCP en `POST /mcp`. No requiere autenticación.

---

## VS Code y Visual Studio (archivo único)

Tanto VS Code como Visual Studio 2022/2026 leen el archivo `%USERPROFILE%\.mcp.json`.

Añade la entrada `ia-recuerdo`:

```json
{
  "inputs": [],
  "servers": {
    "ia-recuerdo": {
      "type": "http",
      "url": "http://<HOST>:7438/mcp"
    }
  }
}
```

Reinicia el cliente para aplicar los cambios.

---

## Claude Code / Cursor / Otros

Añade al archivo de configuración MCP de tu cliente:

```json
{
  "ia-recuerdo": {
    "type": "http",
    "url": "http://<HOST>:7438/mcp"
  }
}
```

---

## Verificar conectividad

```bash
curl -s -X POST http://<HOST>:7438/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}'
# → {"jsonrpc":"2.0","id":1,"result":{"status":"pong"}}
```

---

## Crear API Key (solo para REST API `/api/v1/*`)

El endpoint MCP no necesita auth. Para usar la REST API:

```bash
./ia-recuerdo -create-token "mi-cliente"
# → ir_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Usar en REST:
curl http://<HOST>:7438/api/v1/search?q=test \
  -H "X-Api-Key: ir_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```