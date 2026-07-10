use axum::{
    extract::{
        State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    response::IntoResponse,
};
use tokio::sync::broadcast;

use crate::messages::{ControlMessage, EventMessage};

#[derive(Clone)]
pub struct WsState {
    pub events_tx: broadcast::Sender<String>,
    pub control_tx: broadcast::Sender<String>,
}

impl WsState {
    pub fn new() -> Self {
        let (events_tx, _) = broadcast::channel::<String>(256);
        let (control_tx, _) = broadcast::channel::<String>(256);
        Self {
            events_tx,
            control_tx,
        }
    }
}

pub async fn ws_handler(ws: WebSocketUpgrade, State(state): State<WsState>) -> impl IntoResponse {
    ws.on_upgrade(|socket| handle_socket(socket, state))
}

async fn handle_socket(mut socket: WebSocket, state: WsState) {
    let mut events_rx = state.events_tx.subscribe();
    let control_tx = state.control_tx.clone();
    let events_tx = state.events_tx.clone();

    loop {
        tokio::select! {
            Ok(msg) = events_rx.recv() => {
                if socket.send(Message::Text(msg.into())).await.is_err() {
                    break;
                }
            }

            Some(Ok(msg)) = socket.recv() => {
                match msg {
                    Message::Text(text) => {
                        match serde_json::from_str::<ControlMessage>(&text) {
                            Ok(ctrl) => {
                                // Send ack back immediately
                                let ack = EventMessage::ack(&ctrl.id, true, None);
                                if let Ok(json) = serde_json::to_string(&ack) {
                                    let _ = events_tx.send(json);
                                }
                                // Forward to session handler
                                if let Ok(json) = serde_json::to_string(&ctrl) {
                                    let _ = control_tx.send(json);
                                }
                            }
                            Err(e) => {
                                // Send a failed ack with the parse error
                                let ack = EventMessage::ack("unknown", false, Some(e.to_string()));
                                if let Ok(json) = serde_json::to_string(&ack) {
                                    let _ = events_tx.send(json);
                                }
                            }
                        }
                    }
                    Message::Close(_) => break,
                    _ => {}
                }
            }

            else => break,
        }
    }

    tracing::info!("WebSocket client disconnected");
}
