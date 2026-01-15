const FileFlow = (function () {
    'use strict';

    const CHUNK_SIZE = 4096;

    let ws = null;
    let reconnectAttempts = 0;
    let isOnline = false;
    let activeMessages = new Map();

    const $app = document.getElementById('app');
    const $viewSecret = document.getElementById('view-secret');
    const $viewMain = document.getElementById('view-main');
    const $presenceDot = document.querySelector('.presence-dot');
    const $presenceText = document.getElementById('presence-text');
    const $secretForm = document.getElementById('secret-form');
    const $secretInput = document.getElementById('secret-input');
    const $secretError = document.getElementById('secret-error');
    const $messageStream = document.getElementById('message-stream');
    const $composerInput = document.getElementById('composer-input');
    const $sendButton = document.getElementById('send-button');

    async function init() {
        try {
            const ticketOk = await ensureDeviceTicket();
            if (!ticketOk) return;

            const res = await fetch('/api/session');
            if (res.ok) {
                const data = await res.json();
                if (data.authed) {
                    showView('main');
                    connectWebSocket();
                    setupComposer();
                    return;
                }
            }
            showView('secret');
            setupSecretForm();
        } catch (err) {
            console.error('Initialization failed:', err);
            showView('secret');
            setupSecretForm();
        }
    }

    function showView(view) {
        $viewSecret.style.display = 'none';
        $viewMain.style.display = 'none';
        const $viewUnauthorized = document.getElementById('view-unauthorized');
        if ($viewUnauthorized) $viewUnauthorized.style.display = 'none';

        switch (view) {
            case 'secret':
                $viewSecret.style.display = 'flex';
                break;
            case 'main':
                $viewMain.style.display = 'flex';
                break;
            case 'unauthorized':
                if ($viewUnauthorized) $viewUnauthorized.style.display = 'flex';
                break;
        }
    }

    let identityPromise = null;
    let ticketPromise = null;

    function base64UrlEncode(buffer) {
        const bytes = new Uint8Array(buffer);
        let binary = '';
        for (const b of bytes) binary += String.fromCharCode(b);
        return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
    }

    function base64UrlDecode(input) {
        let str = input.replace(/-/g, '+').replace(/_/g, '/');
        while (str.length % 4) str += '=';
        const binary = atob(str);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
        return bytes;
    }

    function openKeyDB() {
        return new Promise((resolve, reject) => {
            const request = indexedDB.open('fileflow', 1);
            request.onupgradeneeded = () => {
                const db = request.result;
                if (!db.objectStoreNames.contains('keys')) {
                    db.createObjectStore('keys', { keyPath: 'id' });
                }
            };
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
    }

    function idbGet(db, store, key) {
        return new Promise((resolve, reject) => {
            const tx = db.transaction(store, 'readonly');
            const req = tx.objectStore(store).get(key);
            req.onsuccess = () => resolve(req.result || null);
            req.onerror = () => reject(req.error);
        });
    }

    function idbPut(db, store, value) {
        return new Promise((resolve, reject) => {
            const tx = db.transaction(store, 'readwrite');
            const req = tx.objectStore(store).put(value);
            req.onsuccess = () => resolve();
            req.onerror = () => reject(req.error);
        });
    }

    async function getOrCreateIdentity() {
        if (identityPromise) return identityPromise;
        identityPromise = (async () => {
            const db = await openKeyDB();
            let record = await idbGet(db, 'keys', 'device');

            if (!record || !record.publicKey || !record.privateKey) {
                const keyPair = await crypto.subtle.generateKey(
                    { name: 'ECDSA', namedCurve: 'P-256' },
                    true,
                    ['sign', 'verify']
                );
                record = { id: 'device', publicKey: keyPair.publicKey, privateKey: keyPair.privateKey };
                await idbPut(db, 'keys', record);
            }

            const jwk = await crypto.subtle.exportKey('jwk', record.publicKey);
            const canonical = JSON.stringify({ kty: jwk.kty, crv: jwk.crv, x: jwk.x, y: jwk.y });
            const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(canonical));
            const deviceId = base64UrlEncode(hash);

            return {
                deviceId,
                publicJwk: { kty: jwk.kty, crv: jwk.crv, x: jwk.x, y: jwk.y },
                privateKey: record.privateKey
            };
        })();

        try {
            return await identityPromise;
        } finally {
            identityPromise = null;
        }
    }

    async function ensureDeviceTicket() {
        if (ticketPromise) return ticketPromise;
        ticketPromise = (async () => {
            const identity = await getOrCreateIdentity();

            const challengeRes = await fetch('/api/device/challenge', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify({
                    device_id: identity.deviceId,
                    pub_jwk: identity.publicJwk
                })
            });

            if (challengeRes.status === 403) {
                showView('unauthorized');
                const display = document.getElementById('device-id-display');
                if (display) display.textContent = identity.deviceId;
                return false;
            }

            if (!challengeRes.ok) {
                throw new Error('Device challenge failed');
            }

            const challenge = await challengeRes.json();
            const nonceBytes = base64UrlDecode(challenge.nonce);
            const signature = await crypto.subtle.sign(
                { name: 'ECDSA', hash: 'SHA-256' },
                identity.privateKey,
                nonceBytes
            );

            const attestRes = await fetch('/api/device/attest', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify({
                    challenge_id: challenge.challenge_id,
                    device_id: identity.deviceId,
                    signature: base64UrlEncode(signature)
                })
            });

            if (attestRes.status === 403) {
                showView('unauthorized');
                const display = document.getElementById('device-id-display');
                if (display) display.textContent = identity.deviceId;
                return false;
            }

            if (!attestRes.ok) {
                throw new Error('Device attest failed');
            }

            return true;
        })();

        try {
            return await ticketPromise;
        } finally {
            ticketPromise = null;
        }
    }

    function setupSecretForm() {
        $secretForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            $secretError.textContent = '';

            const secret = $secretInput.value;
            if (!secret) return;

            try {
                const ticketOk = await ensureDeviceTicket();
                if (!ticketOk) return;

                const identity = await getOrCreateIdentity();
                const res = await fetch('/api/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'include',
                    body: JSON.stringify({ 
                        secret,
                        device_id: identity.deviceId 
                    })
                });

                const data = await res.json();

                if (res.status === 403 && data.error?.code === 'DEVICE_NOT_ENROLLED') {
                    showView('unauthorized');
                    const display = document.getElementById('device-id-display');
                    if (display) display.textContent = identity.deviceId;
                    return;
                }

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
            checkSessionAndReconnect();
        };

        ws.onerror = (err) => {
            console.error('WebSocket error:', err);
        };
    }

    async function checkSessionAndReconnect() {
        try {
            const ticketOk = await ensureDeviceTicket();
            if (!ticketOk) return;
            const res = await fetch('/api/session');
            if (res.ok) {
                const data = await res.json();
                if (!data.authed) {
                    showView('secret');
                    return;
                }
            }
        } catch (err) {
        }
        scheduleReconnect();
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

        // Add Copy Button
        const copyBtn = document.createElement('button');
        copyBtn.className = 'copy-button';
        copyBtn.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
            </svg>
        `;
        copyBtn.title = "Copy text";
        copyBtn.onclick = async () => {
            const text = Array.from(content.children)
                .map(p => p.textContent)
                .join('\n\n');

            try {
                await navigator.clipboard.writeText(text);
                const originalIcon = copyBtn.innerHTML;
                copyBtn.innerHTML = `
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <polyline points="20 6 9 17 4 12"></polyline>
                    </svg>
                `;
                copyBtn.classList.add('copied');
                setTimeout(() => {
                    copyBtn.innerHTML = originalIcon;
                    copyBtn.classList.remove('copied');
                }, 2000);
            } catch (err) {
                console.error('Failed to copy:', err);
            }
        };
        bubble.appendChild(copyBtn);

        return bubble;
    }

    function scrollToBottom() {
        $messageStream.scrollTop = $messageStream.scrollHeight;
    }

    function setupComposer() {
        $composerInput.addEventListener('input', updateSendButton);

        $composerInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
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
