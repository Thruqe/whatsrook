use std::{env, sync::Arc};
use axum::{
    Router,
    extract::{
        State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    response::IntoResponse,
    routing::get,
};
use tokio::sync::broadcast;
use tower_http::services::ServeDir;
use wacore::{pair_code::{PairCodeOptions, PlatformId}, types::events::Event};
use whatsapp_rust::{TokioRuntime, bot::Bot, store::SqliteStore};
use whatsapp_rust::store::Backend;
use whatsapp_rust_tokio_transport::TokioWebSocketTransportFactory;
use whatsapp_rust_ureq_http_client::UreqHttpClient;

#[derive(Clone)]
struct AppState {
    whatsapp_events_tx: broadcast::Sender<String>,
    whatsapp_control_tx: broadcast::Sender<String>,
}

struct CliArgs {
    session: Option<String>,
    pair: Option<String>,
    qrcode: bool,
    logout: bool,
}

fn parse_args() -> CliArgs {
    let args: Vec<String> = env::args().collect();

    let get_value = |flag: &str| -> Option<String> {
        let index = args.iter().position(|a| a == flag)?;
        args.get(index + 1).cloned()
    };

    CliArgs {
        session: get_value("--session"),
        pair: get_value("--pair"),
        qrcode: args.contains(&"--qrcode".to_string()),
        logout: args.contains(&"--logout".to_string()),
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let port = env::var("PORT").unwrap_or_else(|_| "3000".to_string());

    let (whatsapp_events_tx, _) = broadcast::channel::<String>(256);
    let (whatsapp_control_tx, _) = broadcast::channel::<String>(256);

    let state = AppState {
        whatsapp_events_tx: whatsapp_events_tx.clone(),
        whatsapp_control_tx: whatsapp_control_tx.clone(),
    };

    let serve_dir = ServeDir::new("web")
        .append_index_html_on_directories(true)
        .fallback(tower_http::services::ServeFile::new("web/index.html"));

    let app = Router::new()
        .route("/ws", get(ws_handler))
        .fallback_service(serve_dir)
        .with_state(state);

    let listener = tokio::net::TcpListener::bind(format!("0.0.0.0:{port}")).await?;
    println!("Listening on port {port}");

    let server_handle = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    start_sock(whatsapp_events_tx, whatsapp_control_tx).await?;

    server_handle.await?;
    Ok(())
}

async fn ws_handler(
    ws: WebSocketUpgrade,
    State(state): State<AppState>,
) -> impl IntoResponse {
    ws.on_upgrade(|socket| handle_socket(socket, state))
}

async fn handle_socket(mut socket: WebSocket, state: AppState) {
    println!("WebSocket client connected");

    let mut events_rx = state.whatsapp_events_tx.subscribe();
    let control_tx = state.whatsapp_control_tx.clone();

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
                        let _ = control_tx.send(text.to_string());
                    }
                    Message::Close(_) => break,
                    _ => {}
                }
            }

            else => break,
        }
    }

    println!("WebSocket client disconnected");
}

async fn start_sock(
    events_tx: broadcast::Sender<String>,
    _control_tx: broadcast::Sender<String>,
) -> Result<(), Box<dyn std::error::Error>> {
    let cli = parse_args();

    if cli.logout {
        let session = cli.session.as_deref().unwrap_or("whatsapp");
        println!("Logging out session: {session}");
        let _ = tokio::fs::remove_file(format!("{session}.db")).await;
        println!("Session cleared.");
        return Ok(());
    }

    let db_path = format!("{}.db", cli.session.as_deref().unwrap_or("whatsapp"));
    let backend: Arc<dyn Backend> = Arc::new(SqliteStore::new(&db_path).await?);

    let mut builder = Bot::builder()
        .with_backend(backend)
        .with_transport_factory(TokioWebSocketTransportFactory::new())
        .with_http_client(UreqHttpClient::new())
        .with_runtime(TokioRuntime);

    if let Some(phone) = cli.pair {
        println!("Requesting pair code for: {phone}");
        builder = builder.with_pair_code(PairCodeOptions {
            phone_number: phone,
            show_push_notification: true,
            custom_code: None,
            platform_id: PlatformId::Chrome,
            platform_display: "Chrome (Linux)".to_string(),
        });
    }

    let mut bot = builder
        .on_event(move |event, _client| {
            let tx = events_tx.clone();
            let show_qr = cli.qrcode;
            async move {
                let connection_update_msg = match &event {
                    Event::PairingQrCode { code, .. } => {
                        if show_qr {
                            println!("QR Code:\n{code}");
                        }
                        Some(code.clone())
                    }
                    Event::PairSuccess(_) => {
                        println!("Paired successfully!");
                        Some("Paired successfully!".to_string())
                    }
                    Event::PairError(_) => {
                        println!("Pairing failed.");
                        Some("Pairing failed.".to_string())
                    }
                    Event::LoggedOut(_) => {
                        println!("Logged out.");
                        Some("Connection Logged Out.".to_string())
                    }
                    Event::Disconnected(_) => {
                        Some("Connection closed.".to_string())
                    }
                    _ => None,
                };

                if let Some(msg) = connection_update_msg {
                    let _ = tx.send(msg);
                }

                if let Ok(json) = serde_json::to_string(&event) {
                    let _ = tx.send(json);
                }
            }
        })
        .build()
        .await?;

    bot.run().await?.await?;
    Ok(())
}