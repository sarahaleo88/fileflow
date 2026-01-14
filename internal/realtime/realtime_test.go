package realtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHub(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	t.Run("OnlineCountStartsAtZero", func(t *testing.T) {
		if count := hub.OnlineCount(); count != 0 {
			t.Errorf("Expected 0 clients, got %d", count)
		}
	})
}

func TestHubClientRegistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := NewClient(hub, conn, "test-device", "127.0.0.1", nil, 100)
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect client 1: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if count := hub.OnlineCount(); count != 1 {
		t.Errorf("Expected 1 client, got %d", count)
	}

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect client 2: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if count := hub.OnlineCount(); count != 2 {
		t.Errorf("Expected 2 clients, got %d", count)
	}

	conn1.Close()
	time.Sleep(50 * time.Millisecond)

	if count := hub.OnlineCount(); count != 1 {
		t.Errorf("Expected 1 client after disconnect, got %d", count)
	}

	conn2.Close()
}

func TestPresenceBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := NewClient(hub, conn, "device-"+r.URL.Query().Get("id"), "127.0.0.1", nil, 100)
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn1, _, _ := websocket.DefaultDialer.Dial(wsURL+"?id=1", nil)
	defer conn1.Close()

	time.Sleep(50 * time.Millisecond)

	_, msg, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	var event Event
	json.Unmarshal(msg, &event)

	if event.Type != EventPresence {
		t.Errorf("Expected presence event, got %s", event.Type)
	}
}

func TestMessageForwarding(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := NewClient(hub, conn, "device-"+r.URL.Query().Get("id"), "127.0.0.1", nil, 100)
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn1, _, _ := websocket.DefaultDialer.Dial(wsURL+"?id=1", nil)
	defer conn1.Close()

	conn2, _, _ := websocket.DefaultDialer.Dial(wsURL+"?id=2", nil)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	// Drain presence messages
	// conn1 receives: p1 (self), p2 (conn2)
	conn1.ReadMessage()
	conn1.ReadMessage()
	// conn2 receives: p2 (self)
	conn2.ReadMessage()

	msgStart := Event{
		Type:      EventMsgStart,
		Value:     map[string]interface{}{"msgId": "test-msg-1"},
		Timestamp: time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(msgStart)
	conn1.WriteMessage(websocket.TextMessage, data)

	time.Sleep(50 * time.Millisecond)

	conn2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, received, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive forwarded message: %v", err)
	}

	var receivedEvent Event
	json.Unmarshal(received, &receivedEvent)

	if receivedEvent.Type != EventMsgStart {
		t.Errorf("Expected msg_start, got %s", receivedEvent.Type)
	}
}

func TestSendFailWhenPeerOffline(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := NewClient(hub, conn, "device-solo", "127.0.0.1", nil, 100)
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	conn.ReadMessage()

	msgStart := Event{
		Type:      EventMsgStart,
		Value:     map[string]interface{}{"msgId": "solo-msg"},
		Timestamp: time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(msgStart)
	conn.WriteMessage(websocket.TextMessage, data)

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, received, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive send_fail: %v", err)
	}

	var event Event
	json.Unmarshal(received, &event)

	if event.Type != EventSendFail {
		t.Errorf("Expected send_fail, got %s", event.Type)
	}

	valueMap := event.Value.(map[string]interface{})
	if valueMap["reason"] != "peer_offline" {
		t.Errorf("Expected reason peer_offline, got %v", valueMap["reason"])
	}
}

func TestAckForwarding(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := NewClient(hub, conn, "device-"+r.URL.Query().Get("id"), "127.0.0.1", nil, 100)
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	sender, _, _ := websocket.DefaultDialer.Dial(wsURL+"?id=sender", nil)
	defer sender.Close()

	receiver, _, _ := websocket.DefaultDialer.Dial(wsURL+"?id=receiver", nil)
	defer receiver.Close()

	time.Sleep(100 * time.Millisecond)

	// Drain presence messages
	// sender: p1, p2
	sender.ReadMessage()
	sender.ReadMessage()
	// receiver: p2
	receiver.ReadMessage()

	ack := Event{
		Type:      EventAck,
		Value:     map[string]interface{}{"msgId": "ack-test"},
		Timestamp: time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(ack)
	receiver.WriteMessage(websocket.TextMessage, data)

	sender.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, received, err := sender.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive ack: %v", err)
	}

	var event Event
	json.Unmarshal(received, &event)

	if event.Type != EventAck {
		t.Errorf("Expected ack, got %s", event.Type)
	}
}

func TestConcurrentClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := NewClient(hub, conn, "device", "127.0.0.1", nil, 100)
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	var wg sync.WaitGroup
	connCount := 10

	conns := make([]*websocket.Conn, connCount)

	for i := 0; i < connCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Errorf("Failed to connect: %v", err)
				return
			}
			conns[idx] = conn
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if count := hub.OnlineCount(); count != connCount {
		t.Errorf("Expected %d clients, got %d", connCount, count)
	}

	for _, conn := range conns {
		if conn != nil {
			conn.Close()
		}
	}
}
