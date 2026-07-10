use axum::{
    extract::{
        State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    response::IntoResponse,
};
use tokio::sync::broadcast;

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

    loop {
        tokio::select! {
            Ok(msg) = events_rx.recv() => {
                if socket.send(Message::Text(msg.into())).await.is_err() {
                    break;
                }
            }

            Some(Ok(msg)) = socket.recv() => {
                match msg {
                    Message::Text(text) => { let _ = control_tx.send(text.to_string()); }
                    Message::Close(_) => break,
                    _ => {}
                }
            }

            else => break,
        }
    }

    println!("WebSocket client disconnected");
}
