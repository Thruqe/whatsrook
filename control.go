package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func (b *Bot) handleControl(ctx context.Context, ctrl ControlMessage) EventMessage {
	switch ctrl.Kind {
	case ControlSendMessage:
		return b.handleSendMessage(ctx, ctrl)
	case ControlSendReaction:
		return b.handleSendReaction(ctx, ctrl)
	case ControlEditMessage:
		return b.handleEditMessage(ctx, ctrl)
	case ControlRevokeMessage:
		return b.handleRevokeMessage(ctx, ctrl)
	case ControlGetStatus:
		return b.handleGetStatus(ctrl)
	case ControlDisconnect:
		slog.Info("disconnect requested")
		b.client.Disconnect()
		return ackEvent(ctrl.ID, true, "")
	case ControlLogout:
		slog.Info("logout requested")
		if err := b.client.Logout(ctx); err != nil {
			slog.Error("logout failed", "err", err)
			return ackEvent(ctrl.ID, false, err.Error())
		}
		return ackEvent(ctrl.ID, true, "")
	default:
		slog.Warn("unknown control type", "kind", ctrl.Kind)
		return ackEvent(ctrl.ID, false, "unknown control type")
	}
}

func (b *Bot) handleSendMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p SendMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		slog.Warn("bad send_message payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		slog.Warn("invalid JID", "to", p.To, "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	var msg waE2E.Message
	if p.QuoteID != nil && p.QuoteSender != nil {
		msg = waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: new(p.Text),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    p.QuoteID,
					Participant: p.QuoteSender,
				},
			},
		}
	} else {
		msg = waE2E.Message{Conversation: new(p.Text)}
	}
	resp, err := b.client.SendMessage(ctx, jid, &msg)
	if err != nil {
		slog.Error("send failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	slog.Info("sent", "id", resp.ID)
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) handleSendReaction(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p SendReactionPayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		slog.Warn("bad send_reaction payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		slog.Warn("invalid JID", "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	senderJID := types.EmptyJID
	if p.Sender != nil {
		senderJID, err = types.ParseJID(*p.Sender)
		if err != nil {
			slog.Warn("invalid sender JID", "err", err)
			return ackEvent(ctrl.ID, false, "invalid sender JID: "+err.Error())
		}
	}
	_, err = b.client.SendMessage(ctx, jid, b.client.BuildReaction(jid, senderJID, types.MessageID(p.MessageID), p.Emoji))
	if err != nil {
		slog.Error("reaction failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) handleEditMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p EditMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		slog.Warn("bad edit_message payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		slog.Warn("invalid JID", "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	_, err = b.client.SendMessage(ctx, jid, b.client.BuildEdit(jid, p.MessageID, &waE2E.Message{
		Conversation: new(string),
	}))
	if err != nil {
		slog.Error("edit failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) handleRevokeMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p RevokeMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		slog.Warn("bad revoke_message payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		slog.Warn("invalid JID", "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	var revokeMsg *waE2E.Message
	if p.OriginalSender != nil {
		revokeMsg = b.client.BuildRevoke(jid, types.NewJID(*p.OriginalSender, types.DefaultUserServer), p.MessageID)
	} else {
		revokeMsg = b.client.BuildRevoke(jid, types.EmptyJID, p.MessageID)
	}
	_, err = b.client.SendMessage(ctx, jid, revokeMsg)
	if err != nil {
		slog.Error("revoke failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) handleGetStatus(ctrl ControlMessage) EventMessage {
	connected := b.client.IsConnected()
	loggedIn := b.client.IsLoggedIn()
	slog.Info("status", "connected", connected, "logged_in", loggedIn)
	return EventMessage{
		Kind: EventStatus,
		ID:   &ctrl.ID,
		Payload: map[string]any{
			"connected": connected,
			"logged_in": loggedIn,
		},
	}
}
