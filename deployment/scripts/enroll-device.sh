#!/bin/sh
set -e

if [ -z "$1" ] || [ -z "$2" ]; then
    echo "Usage: $0 <device_id> <label>"
    echo "  Enrolls a device in the whitelist."
    echo ""
    echo "  device_id: The base64url-encoded SHA-256 hash of the device's public key"
    echo "  label: A human-readable name for the device (e.g., 'MacBook Pro')"
    exit 1
fi

DEVICE_ID="$1"
LABEL="$2"
DB_PATH="${SQLITE_PATH:-/data/fileflow.db}"
CREATED_AT=$(date +%s)000

sqlite3 "$DB_PATH" "INSERT INTO devices (device_id, pub_jwk_json, label, created_at) VALUES ('$DEVICE_ID', '{}', '$LABEL', $CREATED_AT);"

echo "Device enrolled successfully:"
echo "  ID: $DEVICE_ID"
echo "  Label: $LABEL"
