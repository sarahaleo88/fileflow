const FileFlow = (function() {
    'use strict';

    const CHUNK_SIZE = 4096;
    const DB_NAME = 'fileflow';
    const DB_VERSION = 1;
    const STORE_NAME = 'keys';

    let db = null;
    let keypair = null;
    let deviceId = null;
    let ws = null;
    let reconnectAttempts = 0;
    let isOnline = false;
    let activeMessages = new Map();

    const $app = document.getElementById('app');
    const $viewUnauthorized = document.getElementById('view-unauthorized');
    const $viewSecret = document.getElementById('view-secret');
    const $viewMain = document.getElementById('view-main');
    const $presenceDot = document.querySelector('.presence-dot');
    const $presenceText = document.getElementById('presence-text');
    const $deviceIdDisplay = document.getElementById('device-id-display');
    const $secretForm = document.getElementById('secret-form');
    const $secretInput = document.getElementById('secret-input');
    const $secretError = document.getElementById('secret-error');
    const $messageStream = document.getElementById('message-stream');
    const $composerInput = document.getElementById('composer-input');
    const $sendButton = document.getElementById('send-button');

    async function init() {
        try {
            await initDB();
            keypair = await loadOrCreateKeypair();
            deviceId = await computeDeviceId(keypair.publicKey);
            
            if ($deviceIdDisplay) {
                $deviceIdDisplay.textContent = deviceId;
            }

            await authenticate();
        } catch (err) {
            console.error('Initialization failed:', err);
        }
    }

    async function initDB() {
        return new Promise((resolve, reject) => {
            const request = indexedDB.open(DB_NAME, DB_VERSION);
            
            request.onerror = () => reject(request.error);
            request.onsuccess = () => {
                db = request.result;
                resolve();
            };
            
            request.onupgradeneeded = (event) => {
                const database = event.target.result;
                if (!database.objectStoreNames.contains(STORE_NAME)) {
                    database.createObjectStore(STORE_NAME, { keyPath: 'id' });
                }
            };
        });
    }

    async function loadOrCreateKeypair() {
        const stored = await loadKeypair();
        if (stored) return stored;

        const kp = await crypto.subtle.generateKey(
            { name: 'ECDSA', namedCurve: 'P-256' },
            false,
            ['sign', 'verify']
        );

        await saveKeypair(kp);
        return kp;
    }

    async function loadKeypair() {
        return new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readonly');
            const store = tx.objectStore(STORE_NAME);
            const request = store.get('keypair');

            request.onerror = () => reject(request.error);
            request.onsuccess = () => {
                if (request.result) {
                    resolve(request.result.value);
                } else {
                    resolve(null);
                }
            };
        });
    }

    async function saveKeypair(kp) {
        return new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readwrite');
            const store = tx.objectStore(STORE_NAME);
            const request = store.put({ id: 'keypair', value: kp });

            request.onerror = () => reject(request.error);
            request.onsuccess = () => resolve();
        });
    }

    async function computeDeviceId(publicKey) {
        const jwk = await crypto.subtle.exportKey('jwk', publicKey);
        const canonical = JSON.stringify({ crv: jwk.crv, kty: jwk.kty, x: jwk.x, y: jwk.y });
        const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(canonical));
        return base64UrlEncode(new Uint8Array(hash));
    }

    async function getPublicKeyJWK() {
        const jwk = await crypto.subtle.exportKey('jwk', keypair.publicKey);
        return { kty: jwk.kty, crv: jwk.crv, x: jwk.x, y: jwk.y };
    }

    function base64UrlEncode(buffer) {
        const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
        let binary = '';
        for (const byte of bytes) {
            binary += String.fromCharCode(byte);
        }
        return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    }

    function base64UrlDecode(str) {
        str = str.replace(/-/g, '+').replace(/_/g, '/');
        while (str.length % 4) str += '=';
        const binary = atob(str);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes;
    }

    async function authenticate() {
        try {
            const pubJwk = await getPublicKeyJWK();
            
            const challengeRes = await fetch('/api/device/challenge', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_id: deviceId, pub_jwk: pubJwk })
            });

            if (!challengeRes.ok) {
                throw new Error('Challenge request failed');
            }

            const challengeData = await challengeRes.json();
            const nonce = base64UrlDecode(challengeData.data.nonce);

            const signature = await crypto.subtle.sign(
                { name: 'ECDSA', hash: 'SHA-256' },
                keypair.privateKey,
                nonce
            );

            const sigBytes = new Uint8Array(signature);
            const sigBase64 = base64UrlEncode(sigBytes);

            const attestRes = await fetch('/api/device/attest', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify({
                    challenge_id: challengeData.data.challenge_id,
                    device_id: deviceId,
                    signature: sigBase64
                })
            });

            const attestData = await attestRes.json();

            if (!attestData.device_ok) {
                showView('unauthorized');
                return;
            }

            showView('secret');
            setupSecretForm();

        } catch (err) {
            console.error('Authentication failed:', err);
            showView('unauthorized');
        }
    }

    function showView(view) {
        $viewUnauthorized.style.display = 'none';
        $viewSecret.style.display = 'none';
        $viewMain.style.display = 'none';

        switch (view) {
            case 'unauthorized':
                $viewUnauthorized.style.display = 'flex';
                break;
            case 'secret':
                $viewSecret.style.display = 'flex';
                break;
            case 'main':
                $viewMain.style.display = 'flex';
                break;
        }
    }

    function setupSecretForm() {
        $secretForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            $secretError.textContent = '';

            const secret = $secretInput.value;
            if (!secret) return;

            try {
                const res = await fetch('/api/auth/secret', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'include',
                    body: JSON.stringify({ secret })
                });

                const data = await res.json();

                if (data.authed) {
                    showView('main');
                    connectWebSocket();
                    setupComposer();
                } else {
                    $secretError.textContent = 'Invalid secret. Please try again.';
                    $secretInput.value = '';
                    $secretInput.focus();
                }
            } catch (err) {
                console.error('Secret auth failed:', err);
                $secretError.textContent = 'Authentication failed. Please try again.';
            }
        });
    }

    function connectWebSocket() {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(`${protocol}//${location.host}/ws`);

        ws.onopen = () => {
            reconnectAttempts = 0;
            updatePresence(1, 2);
        };

        ws.onmessage = (event) => {
            const lines = event.data.split('\n');
            for (const line of lines) {
                if (line.trim()) {
                    handleEvent(JSON.parse(line));
                }
            }
        };

        ws.onclose = () => {
            updatePresence(0, 2);
            scheduleReconnect();
        };

        ws.onerror = (err) => {
            console.error('WebSocket error:', err);
        };
    }

    function scheduleReconnect() {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000);
        const jitter = delay * 0.2 * (Math.random() - 0.5);
        reconnectAttempts++;

        setTimeout(() => {
            if (ws.readyState === WebSocket.CLOSED) {
                connectWebSocket();
            }
        }, delay + jitter);
    }

    function handleEvent(event) {
        switch (event.t) {
            case 'presence':
                updatePresence(event.v.online, event.v.required);
                break;
            case 'msg_start':
                handleMsgStart(event);
                break;
            case 'para_start':
                handleParaStart(event);
                break;
            case 'para_chunk':
                handleParaChunk(event);
                break;
            case 'para_end':
                handleParaEnd(event);
                break;
            case 'msg_end':
                handleMsgEnd(event);
                break;
            case 'ack':
                handleAck(event);
                break;
            case 'send_fail':
                handleSendFail(event);
                break;
        }
    }

    function updatePresence(online, required) {
        isOnline = online >= required;
        
        $presenceDot.classList.remove('online', 'partial');
        if (online >= required) {
            $presenceDot.classList.add('online');
            $presenceText.textContent = 'Connected';
        } else if (online > 0) {
            $presenceDot.classList.add('partial');
            $presenceText.textContent = `Waiting for peer (${online}/${required})`;
        } else {
            $presenceText.textContent = 'Disconnected';
        }

        updateSendButton();
    }

    function handleMsgStart(event) {
        const msgId = event.v.msgId;
        const bubble = createMessageBubble(msgId, 'received');
        $messageStream.appendChild(bubble);
        activeMessages.set(msgId, { bubble, paragraphs: [] });
        scrollToBottom();
    }

    function handleParaStart(event) {
        const state = activeMessages.get(event.v.msgId);
        if (!state) return;

        const para = document.createElement('div');
        para.className = 'paragraph';
        para.dataset.index = event.v.i;
        state.bubble.querySelector('.message-content').appendChild(para);
        state.paragraphs[event.v.i] = para;
    }

    function handleParaChunk(event) {
        const state = activeMessages.get(event.v.msgId);
        if (!state) return;

        const para = state.paragraphs[event.v.i];
        if (para) {
            para.textContent += event.v.s;
            scrollToBottom();
        }
    }

    function handleParaEnd(event) {
    }

    function handleMsgEnd(event) {
        const state = activeMessages.get(event.v.msgId);
        if (!state) return;

        sendEvent('ack', { msgId: event.v.msgId });
        activeMessages.delete(event.v.msgId);
    }

    function handleAck(event) {
        const bubble = document.querySelector(`[data-msg-id="${event.v.msgId}"]`);
        if (bubble) {
            const status = bubble.querySelector('.message-status');
            if (status) {
                status.textContent = 'Delivered';
            }
        }
    }

    function handleSendFail(event) {
        const bubble = document.querySelector(`[data-msg-id="${event.v.msgId}"]`);
        if (bubble) {
            const status = bubble.querySelector('.message-status');
            if (status) {
                status.textContent = event.v.reason === 'peer_offline' ? 'Peer offline' : 'Failed';
                status.classList.add('error');
            }
        }
    }

    function createMessageBubble(msgId, type) {
        const bubble = document.createElement('div');
        bubble.className = `message-bubble ${type}`;
        bubble.dataset.msgId = msgId;

        const content = document.createElement('div');
        content.className = 'message-content';
        bubble.appendChild(content);

        if (type === 'sent') {
            const status = document.createElement('div');
            status.className = 'message-status';
            status.textContent = 'Sending...';
            bubble.appendChild(status);
        }

        return bubble;
    }

    function scrollToBottom() {
        $messageStream.scrollTop = $messageStream.scrollHeight;
    }

    function setupComposer() {
        $composerInput.addEventListener('input', updateSendButton);
        
        $composerInput.addEventListener('keydown', (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                e.preventDefault();
                sendMessage();
            }
        });

        $sendButton.addEventListener('click', sendMessage);
    }

    function updateSendButton() {
        const hasText = $composerInput.value.trim().length > 0;
        $sendButton.disabled = !hasText || !isOnline;
    }

    async function sendMessage() {
        const text = $composerInput.value.trim();
        if (!text || !isOnline) return;

        $composerInput.value = '';
        updateSendButton();

        const msgId = crypto.randomUUID();
        const paragraphs = parseParagraphs(text);

        const bubble = createMessageBubble(msgId, 'sent');
        const content = bubble.querySelector('.message-content');
        
        for (const para of paragraphs) {
            const p = document.createElement('div');
            p.className = 'paragraph';
            p.textContent = para;
            content.appendChild(p);
        }

        $messageStream.appendChild(bubble);
        scrollToBottom();

        sendEvent('msg_start', { msgId });

        for (let i = 0; i < paragraphs.length; i++) {
            sendEvent('para_start', { msgId, i });

            const chunks = chunkText(paragraphs[i]);
            for (const chunk of chunks) {
                sendEvent('para_chunk', { msgId, i, s: chunk });
            }

            sendEvent('para_end', { msgId, i });
        }

        sendEvent('msg_end', { msgId });
    }

    function parseParagraphs(text) {
        return text.split(/\n\n+/).filter(p => p.length > 0);
    }

    function* chunkText(text) {
        for (let i = 0; i < text.length; i += CHUNK_SIZE) {
            yield text.slice(i, i + CHUNK_SIZE);
        }
    }

    function sendEvent(type, value) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            const event = { t: type, v: value, ts: Date.now() };
            ws.send(JSON.stringify(event));
        }
    }

    return { init };
})();

document.addEventListener('DOMContentLoaded', FileFlow.init);
