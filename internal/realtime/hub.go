package realtime

import (
	"log"
	"sync"
)

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	stopCh     chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stopCh:     make(chan struct{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.broadcastPresence()
			log.Printf("Client connected: %s (total: %d)", client.DeviceID, h.OnlineCount())

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.broadcastPresence()
			log.Printf("Client disconnected: %s (total: %d)", client.DeviceID, h.OnlineCount())

		case <-h.stopCh:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
		}
	}
}

func (h *Hub) Stop() {
	close(h.stopCh)
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) broadcastPresence() {
	event := NewEvent(EventPresence, PresenceValue{
		Online:   h.OnlineCount(),
		Required: 2,
	})

	data, err := event.Marshal()
	if err != nil {
		log.Printf("Failed to marshal presence event: %v", err)
		return
	}

	h.Broadcast(data, nil)
}

func (h *Hub) Broadcast(message []byte, exclude *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client == exclude {
			continue
		}
		select {
		case client.send <- message:
		default:
			go func(c *Client) {
				h.unregister <- c
			}(client)
		}
	}
}

func (h *Hub) SendToPeer(sender *Client, message []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client != sender {
			select {
			case client.send <- message:
				return true
			default:
				continue
			}
		}
	}
	return false
}

func (h *Hub) HasPeer(sender *Client) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client != sender {
			return true
		}
	}
	return false
}
