use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ControlType {
    SendMessage,
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
    Ack,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum Payload {
    SendMessage { to: String, text: String },
    PairQr { code: String },
    PairCode { code: String, expires_in: u64 },
    PairError { reason: String },
    Ack { ok: bool, error: Option<String> },
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
