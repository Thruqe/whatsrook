package main

import "encoding/json"

type ControlType string

const (
	ControlSendMessage   ControlType = "send_message"
	ControlSendReaction  ControlType = "send_reaction"
	ControlEditMessage   ControlType = "edit_message"
	ControlRevokeMessage ControlType = "revoke_message"
	ControlDisconnect    ControlType = "disconnect"
	ControlLogout        ControlType = "logout"
	ControlGetStatus     ControlType = "get_status"
)

type EventType string

const (
	EventPairQR          EventType = "pair_qr"
	EventPairCode        EventType = "pair_code"
	EventPairSuccess     EventType = "pair_success"
	EventPairError       EventType = "pair_error"
	EventLoggedOut       EventType = "logged_out"
	EventDisconnected    EventType = "disconnected"
	EventConnected       EventType = "connected"
	EventIncomingMessage EventType = "message"
	EventIncomingCall    EventType = "incoming_call"
	EventAck             EventType = "ack"
	EventStatus          EventType = "status"
)

// ControlMessage is what clients send in to control the bot.
type ControlMessage struct {
	Kind    ControlType     `json:"type"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// EventMessage is what the bot sends out to clients.
type EventMessage struct {
	Kind    EventType `json:"type"`
	ID      *string   `json:"id,omitempty"`
	Payload any       `json:"payload"`
}

func ackEvent(id string, ok bool, errMsg string) EventMessage {
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	return EventMessage{
		Kind: EventAck,
		ID:   &id,
		Payload: map[string]any{
			"ok":    ok,
			"error": e,
		},
	}
}

func simpleEvent(kind EventType) EventMessage {
	return EventMessage{Kind: kind, Payload: map[string]any{}}
}

// Typed payload structs for decoding control messages.

type SendMessagePayload struct {
	To          string  `json:"to"`
	Text        string  `json:"text"`
	QuoteID     *string `json:"quote_id,omitempty"`
	QuoteSender *string `json:"quote_sender,omitempty"`
}

type SendReactionPayload struct {
	To        string  `json:"to"`
	MessageID string  `json:"message_id"`
	Sender    *string `json:"sender,omitempty"`
	Emoji     string  `json:"emoji"`
}

type EditMessagePayload struct {
	To        string `json:"to"`
	MessageID string `json:"message_id"`
	NewText   string `json:"new_text"`
}

type RevokeMessagePayload struct {
	To             string  `json:"to"`
	MessageID      string  `json:"message_id"`
	OriginalSender *string `json:"original_sender,omitempty"`
}

// TO DO: Add more payloads