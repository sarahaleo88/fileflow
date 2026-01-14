const FileFlow = (function() {
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

        switch (view) {
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
                const res = await fetch('/api/login', {
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
            checkSessionAndReconnect();
        };

        ws.onerror = (err) => {
            console.error('WebSocket error:', err);
        };
    }

    async function checkSessionAndReconnect() {
        try {
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
