use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ControlType {
    SendMessage,
    SendReaction,
    EditMessage,
    RevokeMessage,
    Disconnect,
    Logout,
    GetStatus,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EventType {
    PairQr,
    PairCode,
    PairSuccess,
    PairError,
    LoggedOut,
    Disconnected,
    Connected,
    Message,
    Ack,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum Payload {
    SendMessage {
        to: String,
        text: String,
        /// Optional message ID to quote/reply to
        #[serde(skip_serializing_if = "Option::is_none")]
        quote_id: Option<String>,
        /// JID of the original sender (required when quote_id is set)
        #[serde(skip_serializing_if = "Option::is_none")]
        quote_sender: Option<String>,
    },
    SendReaction {
        to: String,
        message_id: String,
        /// JID of the original sender; required for groups, None for DMs
        #[serde(skip_serializing_if = "Option::is_none")]
        sender: Option<String>,
        /// Emoji to react with; empty string removes the reaction
        emoji: String,
    },
    EditMessage {
        to: String,
        message_id: String,
        new_text: String,
    },
    RevokeMessage {
        to: String,
        message_id: String,
        /// For admin-delete in groups: JID of the original sender
        #[serde(skip_serializing_if = "Option::is_none")]
        original_sender: Option<String>,
    },

    PairQr {
        code: String,
    },
    PairCode {
        code: String,
        expires_in: u64,
    },
    PairError {
        reason: String,
    },
    IncomingMessage {
        from: String,
        text: String,
        message_id: String,
    },
    SentMessage {
        to: String,
        message_id: String,
    },
    Ack {
        ok: bool,
        error: Option<String>,
    },
    Empty {},
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ControlMessage {
    #[serde(rename = "type")]
    pub kind: ControlType,
    pub id: String,
    pub payload: Payload,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EventMessage {
    #[serde(rename = "type")]
    pub kind: EventType,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub id: Option<String>,
    pub payload: Payload,
}

impl EventMessage {
    pub fn ack(id: impl Into<String>, ok: bool, error: Option<String>) -> Self {
        Self {
            kind: EventType::Ack,
            id: Some(id.into()),
            payload: Payload::Ack { ok, error },
        }
    }

    pub fn event(kind: EventType, payload: Payload) -> Self {
        Self {
            kind,
            id: None,
            payload,
        }
    }
}
