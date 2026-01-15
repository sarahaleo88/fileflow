# FileFlow User Tutorial

Welcome to FileFlow! This guide will walk you through setting up, running, and using the application locally.

## 1. Quick Start (Local Development)

The easiest way to run the application locally is using `go run` with the necessary environment variables.

### Start the Server

Run the following command in your terminal:

```bash
# Set up a known password hash for "password"
export APP_SECRET_HASH='$argon2id$v=19$m=65536,t=1,p=4$pi5cJJJcFpPz13GfgpbQew$xv/voQ57Uj+SL81Clyda/9gD1mH+ASOWNAQno49GhSQ'

# Run the server
SQLITE_PATH=./dev.db \
FF_DEV=1 \
SECURE_COOKIES=false \
SESSION_KEY=dev-secret \
BOOTSTRAP_TOKEN=admin123 \
go run ./cmd/server/main.go
```

The server will start on `http://localhost:8080`.

## 2. Enrolling Devices

FileFlow uses a strict allowlist system. **New devices cannot log in until they are explicitly enrolled by an administrator.**

### Step A: Get Your Device ID

1. Open your browser to [http://localhost:8080](http://localhost:8080).
2. You will see an **"Unauthorized Device"** screen.
3. Open the Developer Tools (F12 or Right Click -> Inspect).
4. Go to the **Console** tab.
5. You should see a log entry looking like this:
   ```
   Device ID: mbyaYd8cnt_WYV8ET7cWDPcnzvwnuFhxWHySCOS9XMo
   ```
   (Alternatively, check the Network tab for the failed request to `/challenge` - the Request Payload contains the `device_id`).

### Step B: Enroll the Device

Open a new terminal window (keep the server running) and run this command:

```bash
# RSYNC this command with your actual Device ID
curl -X POST http://localhost:8080/api/admin/devices \
  -H "X-Admin-Bootstrap: admin123" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "PASTE_YOUR_DEVICE_ID_HERE",
    "pub_jwk": {"kty":"EC","...copy full jwk object from console if needed, usually just ID is enough for lookup if implemented..."}, 
    "label": "My Local Browser"
  }'
```

*Note: For the current implementation, you need the full `pub_jwk`. The easiest way to get the full JSON payload for enrollment is to copy it from the `Network` tab in DevTools:*

1. In DevTools > Network, find the red (failed) `challenge` request.
2. Click it and view the **Payload** (or Request Body).
3. Copy the entire JSON object (`device_id` and `pub_jwk`).
4. Use that in your curl command:

```bash
curl -X POST http://localhost:8080/api/admin/devices \
  -H "X-Admin-Bootstrap: admin123" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "...", 
    "pub_jwk": {...}, 
    "label": "My Setup"
  }'
```

*(If you are stuck, you can also use `scripts/enroll-device.sh` if you have `sqlite3` installed locally, but the API method above is universal).*

## 3. Logging In

1. Refresh the page at `http://localhost:8080`.
2. The "Unauthorized" screen should disappear.
3. Enter the shared secret: `password`
4. Click **Connect**.

You should now see the main chat interface with a status of "waiting for peer".

## 4. Connecting a Second Device

To test messaging, you need a second device (or a private/incognito window).

1. Open a **Private/Incognito** window (this creates a fresh "device" identity).
2. Go to `http://localhost:8080`.
3. You will see "Unauthorized Device" again (because this is a new identity).
4. Repeat **Step 2** (Enrollment) for this new device ID.
5. Login with `password`.

## 5. Sending Messages

Once both devices are logged in:
1. The status indicator should turn **Green** ("Connected").
2. Type a message in the input box at the bottom.
3. Press **Enter** to send.
4. The text will stream in real-time to the other window.

## Troubleshooting

- **"Context deadline exceeded" / Database Locked**: If you stop the server abruptly, the SQLite file might be locked. Restart the server.
- **"Unauthorized Device" loop**: Ensure you copied the `device_id` exactly and that the server didn't restart with a different database file (unless you are using persistent `./dev.db`).
- **Resetting everything**: delete `dev.db` and clear your browser's "Application > Storage" (or IndexedDB) to start fresh.
