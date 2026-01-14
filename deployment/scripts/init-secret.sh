#!/bin/sh
set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <secret>"
    echo "  Generates an Argon2id hash and stores it in the database."
    exit 1
fi

SECRET="$1"
DB_PATH="${SQLITE_PATH:-/data/fileflow.db}"

HASH=$(./fileflow hash-secret "$SECRET" 2>/dev/null || echo "")

if [ -z "$HASH" ]; then
    echo "Note: Using sqlite3 fallback for hash storage."
    echo "Please set the secret_hash manually or rebuild with hash-secret command."
    
    read -p "Enter pre-computed Argon2id hash: " HASH
fi

sqlite3 "$DB_PATH" "INSERT INTO config (key, value) VALUES ('secret_hash', '$HASH') ON CONFLICT(key) DO UPDATE SET value = excluded.value;"

echo "Secret hash stored successfully."
