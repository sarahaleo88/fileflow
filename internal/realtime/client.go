package realtime

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lixiansheng/fileflow/internal/limit"
	"golang.org/x/time/rate"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 256 * 1024
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	DeviceID string

	// Rate limiting
	limiter     *rate.Limiter
	connLimiter *limit.ConnLimiter
	ip          string

	mu             sync.Mutex
	activeMessages map[string]*MessageState
}

type MessageState struct {
	MsgID       string
	ParaCount   int
	TotalBytes  int
	CurrentPara int
}

func NewClient(hub *Hub, conn *websocket.Conn, deviceID, ip string, connLimiter *limit.ConnLimiter, rateLimit int) *Client {
	return &Client{
		hub:            hub,
		conn:           conn,
		send:           make(chan []byte, 256),
		DeviceID:       deviceID,
		activeMessages: make(map[string]*MessageState),
		limiter:        rate.NewLimiter(rate.Limit(rateLimit), rateLimit), // Burst = rate
		connLimiter:    connLimiter,
		ip:             ip,
	}
}

func (c *Client) ReadPump() {
	defer func() {
		if c.connLimiter != nil {
			c.connLimiter.Decrement(c.ip)
		}
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		if !c.limiter.Allow() {
			log.Printf("Rate limit exceeded for client %s (%s)", c.DeviceID, c.ip)
			break
		}

		c.handleMessage(message)
	}
}

func (c *Client) handleMessage(data []byte) {
	event, err := ParseEvent(data)
	if err != nil {
		log.Printf("Failed to parse event: %v", err)
		return
	}

	switch event.Type {
	case EventMsgStart:
		c.handleMsgStart(event, data)
	case EventParaStart:
		c.handleParaStart(event, data)
	case EventParaChunk:
		c.handleParaChunk(event, data)
	case EventParaEnd:
		c.handleParaEnd(event, data)
	case EventMsgEnd:
		c.handleMsgEnd(event, data)
	case EventAck:
		c.hub.SendToPeer(c, data)
	}
}

func (c *Client) handleMsgStart(event *Event, data []byte) {
	msgID := event.GetMsgID()
	if msgID == "" {
		return
	}

	if !c.hub.HasPeer(c) {
		c.sendFail(msgID, "peer_offline")
		return
	}

	c.mu.Lock()
	c.activeMessages[msgID] = &MessageState{
		MsgID:       msgID,
		ParaCount:   0,
		TotalBytes:  0,
		CurrentPara: -1,
	}
	c.mu.Unlock()

	c.hub.SendToPeer(c, data)
}

func (c *Client) handleParaStart(event *Event, data []byte) {
	msgID := event.GetMsgID()
	paraIdx := event.GetParaIndex()

	c.mu.Lock()
	state, ok := c.activeMessages[msgID]
	if !ok {
		c.mu.Unlock()
		return
	}

	if paraIdx >= MaxParagraphs {
		c.mu.Unlock()
		c.sendFail(msgID, "max_paragraphs_exceeded")
		return
	}

	state.CurrentPara = paraIdx
	state.ParaCount++
	c.mu.Unlock()

	c.hub.SendToPeer(c, data)
}

func (c *Client) handleParaChunk(event *Event, data []byte) {
	msgID := event.GetMsgID()
	chunkText := event.GetChunkText()

	c.mu.Lock()
	state, ok := c.activeMessages[msgID]
	if !ok {
		c.mu.Unlock()
		return
	}

	chunkLen := len(chunkText)
	if chunkLen > MaxChunkSize {
		c.mu.Unlock()
		c.sendFail(msgID, "chunk_too_large")
		return
	}

	state.TotalBytes += chunkLen
	if state.TotalBytes > MaxMessageSize {
		c.mu.Unlock()
		c.sendFail(msgID, "message_too_large")
		return
	}
	c.mu.Unlock()

	c.hub.SendToPeer(c, data)
}

func (c *Client) handleParaEnd(event *Event, data []byte) {
	msgID := event.GetMsgID()

	c.mu.Lock()
	state, ok := c.activeMessages[msgID]
	if !ok {
		c.mu.Unlock()
		return
	}
	state.CurrentPara = -1
	c.mu.Unlock()

	c.hub.SendToPeer(c, data)
}

func (c *Client) handleMsgEnd(event *Event, data []byte) {
	msgID := event.GetMsgID()

	c.mu.Lock()
	delete(c.activeMessages, msgID)
	c.mu.Unlock()

	c.hub.SendToPeer(c, data)
}

func (c *Client) sendFail(msgID, reason string) {
	event := NewEvent(EventSendFail, SendFailValue{
		MsgID:  msgID,
		Reason: reason,
	})

	data, err := event.Marshal()
	if err != nil {
		return
	}

	select {
	case c.send <- data:
	default:
	}

	c.mu.Lock()
	delete(c.activeMessages, msgID)
	c.mu.Unlock()
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) Send(data []byte) {
	select {
	case c.send <- data:
	default:
	}
}
