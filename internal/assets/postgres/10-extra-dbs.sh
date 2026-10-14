#!/bin/bash
# Runs once, when the Postgres volume is first initialised.
# POSTGRES_EXTRA_DBS (space-separated) entries are either:
#   name                  database owned by POSTGRES_USER
#   name:user:password    login role <user> (created if missing) owning database <name>
set -euo pipefail
q() { psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"; }
for entry in ${POSTGRES_EXTRA_DBS:-}; do
  db=${entry%%:*}
  if [[ "$entry" == *:* ]]; then
    rest=${entry#*:}; user=${rest%%:*}; pass=${rest#*:}
    echo "creating role $user and database $db"
    q -v u="$user" -v p="$pass" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'u', :'p')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'u') \gexec
SQL
    q -c "CREATE DATABASE \"$db\" OWNER \"$user\""
  else
    echo "creating database $db"
    q -c "CREATE DATABASE \"$db\""
  fi
done
