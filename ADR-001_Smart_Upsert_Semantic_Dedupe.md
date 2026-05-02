# ADR-001: Smart Upsert con Deduplicación Semántica Async

**Fecha:** 2026-05-02
**Status:** Aprobado
**Decisor:** Equipo Thiscloud IA

## Contexto y Problema

`mem_save` actualmente inserta sin revisar si existe contenido relacionado o similar. Esto causa:

1. **Duplicación innecesaria**: El mismo bug/patrón se guarda múltiples veces como registros separados
2. **Ruido en búsquedas**: Resultados redundantes contaminan el contexto del agente
3. **Sin relaciones automáticas**: Observaciones relacionadas no se vinculan sólas
4. **Latencia variable**: Buscar antes de guardar puede ser lento con muchos registros

## Decisión

Implementar **"Post-Save Async + Smart Update"**: el `mem_save` inserta inmediatamente y luego, en background (goroutine), detecta similaridad y actúa según threshold.

```
mem_save() → INSERT rápido → retorna inmediatamente
           → Goroutine async:
               ├── Genera embedding
               ├── Busca similares (pgvector)
               ├── Si similarity > 0.92 → UPDATE in-place (mismo topic, contenido evolucionó)
               ├── Si similarity > 0.70 → CREATE relation "related_to"
               └── Si similarity < 0.70 → nada (es realmente nuevo)
```

## Arquitectura

### Flujo

```
┌─────────────┐     INSERT      ┌─────────────┐
│  mem_save   │ ───────────────▶│  PostgreSQL │
│             │   retorna <10ms │             │
└─────────────┘                 └──────┬──────┘
                                       │ async
                                       ▼
                              ┌─────────────────┐
                              │  PostSaveWorker │
                              │  - Embedding    │
                              │  - findSimilar   │
                              │  - threshold     │
                              └────────┬────────┘
                                       │
              ┌────────────────────────┼────────────────────────┐
              ▼                        ▼                        ▼
      similarity > 0.92        similarity > 0.70        similarity < 0.70
              │                        │                        │
              ▼                        ▼                        ▼
      UPDATE existing           CREATE relation           nothing (new)
      (same topic evolved)      "related_to" link
```

### Thresholds configurables

| Threshold | Acción | Uso |
|-----------|--------|-----|
| `SIMILARITY_UPDATE = 0.92` | UPDATE in-place | Mismo tema, contenido evolucionó |
| `SIMILARITY_RELATED = 0.70` | CREATE relation | Tema relacionado pero distinto |
| `SIMILARITY_NONE = 0.70` | No action | Es contenido genuinamente nuevo |

### TopicKey Smart Generation

Si `mem_save` no recibe `topic_key`:
1. Generar automáticamente: `normalize(title) + "-" + type`
2. Normalizar: lowercase, replace spaces with `-`, strip special chars
3. Usar para matching en post-save

## Contratos

### mem_save (actual → modificado)

**Input:**
```go
type Observation struct {
    Title    string   // required
    Content  string   // required
    Type     string   // default: "discovery"
    Project  string   // default: "default"
    Scope    string   // default: "project"
    TopicKey string   // optional, genera automático si nil
    Tags     []string // optional
}
```

**Output (sin cambios):**
```json
{
  "id": 123,
  "status": "saved",
  "message": "Memory saved: \"título\" (type)"
}
```

### Nuevo: PostSaveResult (retorna async, no bloquea)

El worker actualiza embed + detecta similaridad, pero **no retorna al caller original**. El agente puede consultar con `mem_search` o `mem_list_relations` para ver vínculos creados.

### Configuración

```go
type SmartUpsertConfig struct {
    Enabled           bool    // default: true
    ThresholdUpdate   float64 // default: 0.92
    ThresholdRelated  float64 // default: 0.70
    EmbeddingDims     int     // default: 768
    AsyncWorkers      int     // default: 2
}
```

## Métricas y Observabilidad

| Métrica | Descripción |
|---------|-------------|
| `upsert_updates_total` | Count de updates por similarity > threshold |
| `upsert_relations_total` | Count de relaciones creadas |
| `upsert_skipped_total` | Count de inserts sin acción post-save |
| `upsert_latency_ms` | Latencia del worker async |

## Trade-offs

| A favor | En contra |
|---------|-----------|
| Latencia constante <10ms | Requiere pgvector (ya existe) |
| Deduplicación implícita | Encoding puede crecer si no se compacta |
| Relaciones automáticas | Debug harder (operación async) |
| Escala bien con más datos | Workers pueden competir por CPU |

## Alternativas descartadas

1. **Pre-save sync search**: Latencia variable, bloquea agente
2. **Full graph ORM**: Overkill, mayor complejidad
3. **Tabla de versiones separada**: Más storage, misma información

## Checklist de implementación

- [x] Modificar `mem_save` para INSERT inmediato
- [x] Agregar goroutine post-save (postSaveWorker)
- [x] Implementar `findSimilarByEmbedding` con threshold
- [x] Configurar TopicKey smart generation
- [ ] Agregar métricas de upsert (pendiente, no crítico)
- [x] Tests de performance (no regresión)
- [x] Actualizar IA_Recuerdo.md

## Implementación completada

### Tipos (`pkg/types/types.go`)
- `SmartUpsertConfig` con `Enabled`, `ThresholdUpdate`, `ThresholdRelated`, `AsyncWorkers`
- `PostSaveAction` con valores: `updated`, `related`, `none`
- `PostSaveResult` con `ObservationID`, `Action`, `TargetID`, `Similarity`

### Store (`internal/store/store.go`)
- `postSaveCh chan postSaveRequest` — canal no bloqueante
- `startPostSaveWorkers(n)` — lanza N goroutines
- `Close()` — cierra canal y espera workers
- `SaveObservation()` — INSERT inmediato + encola post-save
- `processPostSave(req)` — worker loop
- `findSimilarByEmbedding()` — pgvector similarity, excluye self
- `mergeIntoExisting()` — actualiza target, soft-deletes source
- `SmartTopicKey()` — genera topic_key automático

### Flags y configuración
```bash
-upsert-enabled (default: true)
-upsert-workers (default: 2)
-upsert-thresh-update (default: 0.92)
-upsert-thresh-related (default: 0.70)

# Variables de entorno
IA_UPSERT_ENABLED=true
IA_UPSERT_WORKERS=2
IA_UPSERT_THRESH_UPDATE=0.92
IA_UPSERT_THRESH_RELATED=0.70
```

### Tests (`internal/store/smart_upsert_test.go`)
- `TestSmartUpsertConfig_Defaults`
- `TestSmartTopicKey`
- `TestSmartTopicKey_Max5Words`
- `TestSmartTopicKey_StripsSpecialChars`
- `TestSmartUpsert_SaveObservation_NonBlocking`
- `TestSmartUpsert_CloseClosesChannel`
- `TestSmartUpsert_PostSaveActionConstants`
- `TestSmartUpsert_PostSaveResult`
- `TestStore_New_WithUpsertConfig`

## Status History

- 2026-05-02: Aprobado
- 2026-05-02: Implementación completada
- 2026-05-02: Implementado — tipos, store async workers, CLI flags, tests