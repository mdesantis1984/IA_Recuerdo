#!/usr/bin/env python3
"""
migrate-engram.py — Migra datos de Engram (SQLite) a IA_Recuerdo (PostgreSQL)

Uso:
    python3 migrate-engram.py <ruta_a_engram.db> <postgres_dsn>

Ejemplo:
    python3 migrate-engram.py /tmp/engram.db \
        "postgres://ia_recuerdo:PASSWORD@localhost:5432/ia_recuerdo?sslmode=disable"

Transformaciones:
  sessions:
    - directory    -> descartado (no existe en IA_Recuerdo)
    - agent        -> '' (nuevo campo)
    - goal         -> '' (nuevo campo)

  observations:
    - tool_name        -> descartado
    - sync_id          -> descartado
    - normalized_hash  -> descartado
    - tags             -> '' (nuevo campo)
    - embedding        -> NULL (nuevo campo, requiere calcular vectores aparte)
"""

import sqlite3
import sys
import re
from datetime import timezone
from urllib.parse import urlparse, parse_qs

try:
    import psycopg2
    import psycopg2.extras
except ImportError:
    print("ERROR: instalar psycopg2 → pip install psycopg2-binary", file=sys.stderr)
    sys.exit(1)


def ts(val):
    """Convierte timestamp TEXT de SQLite (UTC naive) a string ISO con +00:00."""
    if val is None:
        return None
    # SQLite guarda como '2026-03-29 12:01:02' — añade timezone UTC para PG
    v = val.strip()
    if "+" not in v and v.endswith("Z") is False and len(v) <= 26:
        return v + "+00:00"
    return v


def migrate_sessions(sqlite_cur, pg_cur):
    sqlite_cur.execute("""
        SELECT id, project, COALESCE(summary, ''), started_at, ended_at
        FROM sessions
    """)
    rows = sqlite_cur.fetchall()
    inserted = 0
    skipped = 0
    errors = 0
    for row in rows:
        sid, project, summary, started_at, ended_at = row
        pg_cur.execute("SAVEPOINT sp")
        try:
            pg_cur.execute("""
                INSERT INTO sessions (id, project, agent, goal, summary, started_at, ended_at)
                VALUES (%s, %s, %s, %s, %s, %s, %s)
                ON CONFLICT (id) DO NOTHING
            """, (sid, project, '', '', summary, ts(started_at), ts(ended_at)))
            if pg_cur.rowcount > 0:
                inserted += 1
            else:
                skipped += 1
            pg_cur.execute("RELEASE SAVEPOINT sp")
        except Exception as e:
            pg_cur.execute("ROLLBACK TO SAVEPOINT sp")
            pg_cur.execute("RELEASE SAVEPOINT sp")
            print(f"  WARN session {sid}: {e}", file=sys.stderr)
            errors += 1
    return inserted, skipped


def migrate_observations(sqlite_cur, pg_cur):
    sqlite_cur.execute("""
        SELECT
            id, session_id,
            COALESCE(type, 'discovery'),
            COALESCE(title, '(sin título)'),
            COALESCE(content, ''),
            COALESCE(project, 'default'),
            COALESCE(scope, 'project'),
            topic_key,
            COALESCE(revision_count, 1),
            COALESCE(duplicate_count, 1),
            COALESCE(last_seen_at, updated_at, created_at, datetime('now')),
            COALESCE(created_at, datetime('now')),
            COALESCE(updated_at, created_at, datetime('now')),
            deleted_at
        FROM observations
    """)
    rows = sqlite_cur.fetchall()
    inserted = 0
    skipped = 0
    errors = 0
    for row in rows:
        (oid, session_id, otype, title, content, project, scope,
         topic_key, revision_count, duplicate_count,
         last_seen_at, created_at, updated_at, deleted_at) = row
        pg_cur.execute("SAVEPOINT sp")
        try:
            pg_cur.execute("""
                INSERT INTO observations (
                    id, session_id, type, title, content, project, scope,
                    topic_key, tags, revision_count, duplicate_count,
                    last_seen_at, created_at, updated_at, deleted_at
                )
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                ON CONFLICT (id) DO NOTHING
            """, (
                oid, session_id, otype, title, content, project, scope,
                topic_key, '',  # tags = ''
                revision_count, duplicate_count,
                ts(last_seen_at), ts(created_at), ts(updated_at), ts(deleted_at)
            ))
            if pg_cur.rowcount > 0:
                inserted += 1
            else:
                skipped += 1
            pg_cur.execute("RELEASE SAVEPOINT sp")
        except Exception as e:
            pg_cur.execute("ROLLBACK TO SAVEPOINT sp")
            pg_cur.execute("RELEASE SAVEPOINT sp")
            print(f"  WARN obs {oid}: {e}", file=sys.stderr)
            errors += 1
    if errors:
        print(f"  ({errors} observaciones saltadas por error)")
    return inserted, skipped


def migrate_prompts(sqlite_cur, pg_cur):
    """Migra user_prompts si la tabla prompts de IA_Recuerdo tiene estructura compatible."""
    sqlite_cur.execute("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='user_prompts'")
    if sqlite_cur.fetchone()[0] == 0:
        return 0, 0

    sqlite_cur.execute("PRAGMA table_info(user_prompts)")
    cols = [r[1] for r in sqlite_cur.fetchall()]
    print(f"  user_prompts columns: {cols}")

    pg_cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name='prompts'")
    pg_cols = [r[0] for r in pg_cur.fetchall()]
    print(f"  prompts (PG) columns: {pg_cols}")

    # Solo migrar si tienen columnas comunes mínimas
    common = set(cols) & set(pg_cols)
    if 'id' not in common or 'content' not in common:
        print("  SKIP: schemas incompatibles para user_prompts → prompts")
        return 0, 0

    sqlite_cur.execute(f"SELECT id, content FROM user_prompts")
    rows = sqlite_cur.fetchall()
    inserted, skipped = 0, 0
    for pid, content in rows:
        try:
            pg_cur.execute(
                "INSERT INTO prompts (id, content) VALUES (%s, %s) ON CONFLICT (id) DO NOTHING",
                (pid, content)
            )
            if pg_cur.rowcount > 0:
                inserted += 1
            else:
                skipped += 1
        except Exception as e:
            print(f"  WARN prompt {pid}: {e}", file=sys.stderr)
    return inserted, skipped


def main():
    if len(sys.argv) != 3:
        print(f"Uso: {sys.argv[0]} <engram.db> <postgres_dsn>")
        sys.exit(1)

    sqlite_path = sys.argv[1]
    pg_dsn = sys.argv[2]

    print(f"[migrate] Abriendo SQLite: {sqlite_path}")
    sqlite_conn = sqlite3.connect(sqlite_path)
    # Checkpoint WAL para asegurar que todos los datos están en el archivo principal
    sqlite_conn.execute("PRAGMA wal_checkpoint(FULL)")
    sqlite_cur = sqlite_conn.cursor()

    print(f"[migrate] Conectando a PostgreSQL...")
    pg_conn = psycopg2.connect(pg_dsn)
    pg_cur = pg_conn.cursor()

    # Estadísticas fuente
    sqlite_cur.execute("SELECT COUNT(*) FROM sessions")
    s_count = sqlite_cur.fetchone()[0]
    sqlite_cur.execute("SELECT COUNT(*) FROM observations")
    o_count = sqlite_cur.fetchone()[0]
    sqlite_cur.execute("SELECT COUNT(*) FROM observations WHERE deleted_at IS NOT NULL")
    o_deleted = sqlite_cur.fetchone()[0]

    print(f"[migrate] Fuente: {s_count} sessions, {o_count} observations ({o_deleted} borradas lógicamente)")

    # Ajustar secuencia antes de insertar
    # (necesario para evitar conflictos de secuencia con IDs numéricos)
    sqlite_cur.execute("SELECT MAX(id) FROM observations")
    max_obs_id = sqlite_cur.fetchone()[0] or 0
    if max_obs_id > 0:
        pg_cur.execute(f"SELECT setval('observations_id_seq', GREATEST(nextval('observations_id_seq'), {max_obs_id + 1}))")

    print("[migrate] Migrando sessions...")
    s_ins, s_skip = migrate_sessions(sqlite_cur, pg_cur)
    pg_conn.commit()
    print(f"  → {s_ins} insertadas, {s_skip} ya existían")

    print("[migrate] Migrando observations...")
    o_ins, o_skip = migrate_observations(sqlite_cur, pg_cur)
    pg_conn.commit()
    print(f"  → {o_ins} insertadas, {o_skip} ya existían")

    print("[migrate] Verificando prompts...")
    p_ins, p_skip = migrate_prompts(sqlite_cur, pg_cur)
    if p_ins + p_skip > 0:
        print(f"  → {p_ins} insertadas, {p_skip} ya existían")

    pg_conn.commit()
    pg_cur.close()
    pg_conn.close()
    sqlite_conn.close()

    print(f"\n[migrate] ✓ Completado: {s_ins} sessions + {o_ins} observations migradas")


if __name__ == "__main__":
    main()
