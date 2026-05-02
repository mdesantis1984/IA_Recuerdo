# Ollama Embeddings Service - CT206

## Descripción

LXC dedicada exclusivamente a servir modelos de embeddings para el ecosistema ThisCloudServices.
Cualquier MCP puede consumir este servicio para generar embeddings vectoriales.

## Información del Contenedor

| Propiedad | Valor |
|-----------|-------|
| CTID | 206 |
| Hostname | ollama-embeddings |
| IP | 10.0.0.206 |
| Puerto Ollama | 11434 |
| Sistema | Debian 12 |
| Cores | 2 |
| Memory | 4GB |
| Almacenamiento | 20GB (local-lvm) |
| Onboot | yes |
| Unprivileged | yes |

## Modelo de Embeddings

| Propiedad | Valor |
|-----------|-------|
| Nombre | nomic-embed-text |
| Versión | latest |
| Tamaño | 274 MB |
| Dimensiones | 768 |

## API Endpoints

### Embeddings

```bash
curl -s http://10.0.0.206:11434/api/embeddings \
  -H "Content-Type: application/json" \
  -d '{"model":"nomic-embed-text","prompt":"texto a embeddear"}'
```

Respuesta:
```json
{"embedding":[0.66543537...,-0.58178341...]}
```

### Generate (no recomendado para embeddings)

```bash
curl -s http://10.0.0.206:11434/api/generate \
  -H "Content-Type: application/json" \
  -d '{"model":"nomic-embed-text","prompt":"texto","stream":false}'
```

## Configuración de Red

Ollama está configurado para escuchar en todas las interfaces:

```
OLLAMA_HOST=0.0.0.0:11434
```

Archivo de override: `/etc/systemd/system/ollama.service.d/override.conf`

## Comandos de Gestión

```bash
# Ver estado del servicio
pct exec 206 -- systemctl status ollama

# Reiniciar Ollama
pct exec 206 -- systemctl restart ollama

# Ver logs
pct exec 206 -- journalctl -u ollama -f

# Listar modelos
pct exec 206 -- ollama list

# Agregar nuevo modelo de embeddings
pct exec 206 -- ollama pull <modelo>
```

## Integración con IA_Recuerdo

Para usar este servicio desde ia-recuerdo u otros MCPs:

```go
// Ejemplo de configuración para cliente Ollama
endpoint := "http://10.0.0.206:11434/v1"
model := "nomic-embed-text"
```

## Notas

- Este CT es exclusivo para embeddings. No ejecutar otros modelos pesados aquí.
- El servicio está configurado para iniciar automáticamente con el CT (`onboot: 1`).
- El modelo nomic-embed-text es óptimo para textos en inglés. Para otros idiomas,
  considerar modelos multilingual como `mxbai-embed-large` o `bge-m3`.