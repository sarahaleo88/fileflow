package realtime

import (
	"encoding/json"
	"time"
)

const (
	EventPresence  = "presence"
	EventMsgStart  = "msg_start"
	EventParaStart = "para_start"
	EventParaChunk = "para_chunk"
	EventParaEnd   = "para_end"
	EventMsgEnd    = "msg_end"
	EventAck       = "ack"
	EventSendFail  = "send_fail"
)

const (
	MaxChunkSize   = 4 * 1024
	MaxMessageSize = 256 * 1024
	MaxParagraphs  = 512
)

type Event struct {
	Type      string      `json:"t"`
	Value     interface{} `json:"v"`
	Timestamp int64       `json:"ts"`
}

type PresenceValue struct {
	Online   int `json:"online"`
	Required int `json:"required"`
}

type MsgStartValue struct {
	MsgID string `json:"msgId"`
}

type ParaStartValue struct {
	MsgID string `json:"msgId"`
	Index int    `json:"i"`
}

type ParaChunkValue struct {
	MsgID string `json:"msgId"`
	Index int    `json:"i"`
	Text  string `json:"s"`
}

type ParaEndValue struct {
	MsgID string `json:"msgId"`
	Index int    `json:"i"`
}

type MsgEndValue struct {
	MsgID string `json:"msgId"`
}

type AckValue struct {
	MsgID string `json:"msgId"`
}

type SendFailValue struct {
	MsgID  string `json:"msgId"`
	Reason string `json:"reason"`
}

func NewEvent(eventType string, value interface{}) *Event {
	return &Event{
		Type:      eventType,
		Value:     value,
		Timestamp: time.Now().UnixMilli(),
	}
}

func (e *Event) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func ParseEvent(data []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (e *Event) GetMsgID() string {
	if e.Value == nil {
		return ""
	}

	valueMap, ok := e.Value.(map[string]interface{})
	if !ok {
		return ""
	}

	msgID, _ := valueMap["msgId"].(string)
	return msgID
}

func (e *Event) GetParaIndex() int {
	if e.Value == nil {
		return -1
	}

	valueMap, ok := e.Value.(map[string]interface{})
	if !ok {
		return -1
	}

	idx, ok := valueMap["i"].(float64)
	if !ok {
		return -1
	}
	return int(idx)
}

func (e *Event) GetChunkText() string {
	if e.Value == nil {
		return ""
	}

	valueMap, ok := e.Value.(map[string]interface{})
	if !ok {
		return ""
	}

	text, _ := valueMap["s"].(string)
	return text
}
