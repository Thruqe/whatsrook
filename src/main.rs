mod cli;
mod messages;
mod utils;
mod ws;

use axum::{Router, routing::get};
use std::{env, sync::Arc};
use wacore::{pair_code::PairCodeOptions, store::DevicePropsOverride, types::events::Event};
use waproto::whatsapp::device_props::PlatformType;
use whatsapp_rust::pair::CompanionWebClientType;
use whatsapp_rust::store::Backend;
use whatsapp_rust::{ClientProfile, TokioRuntime, bot::Bot, store::SqliteStore};
use whatsapp_rust_tokio_transport::TokioWebSocketTransportFactory;
use whatsapp_rust_ureq_http_client::UreqHttpClient;

use cli::CliArgs;
use messages::{ControlMessage, ControlType, EventMessage, EventType, Payload};
use utils::{cleanup_db, shutdown_signal};
use ws::{WsState, ws_handler};

#[tokio::main(name = "zevBot")]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cli = CliArgs::parse();

    let log_level = if cli.debug {
        tracing::Level::DEBUG
    } else {
        tracing::Level::INFO
    };
    tracing_subscriber::fmt()
        .with_max_level(log_level)
        .with_target(cli.debug)
        .init();

    let port = cli
        .port
        .clone()
        .unwrap_or_else(|| env::var("PORT").unwrap_or_else(|_| "3000".to_string()));

    let auth_dir = cli.auth_dir.clone().unwrap_or_else(|| "auth".to_string());

    tokio::fs::create_dir_all(&auth_dir).await?;

    let db_path = format!("{auth_dir}/{}.db", cli.session);

    if cli.logout {
        println!("Logging out session: {}", cli.session);
        for suffix in ["", "-shm", "-wal"] {
            let path = format!("{db_path}{suffix}");
            match tokio::fs::remove_file(&path).await {
                Ok(_) => {}
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => eprintln!("Failed to remove {path}: {e}"),
            }
        }
        println!("Session cleared.");
        return Ok(());
    }

    let db_path_for_signal = db_path.clone();
    tokio::spawn(async move {
        shutdown_signal().await;
        println!("\nShutting down...");
        cleanup_db(&db_path_for_signal).await;
        std::process::exit(0);
    });

    let ws_state = WsState::new();

    let app = Router::new()
        .route("/ws", get(ws_handler))
        .with_state(ws_state.clone());

    let listener = tokio::net::TcpListener::bind(format!("0.0.0.0:{port}")).await?;
    tracing::info!("Listening on port {port} | session: {}", cli.session);

    tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    start_session(db_path, cli, ws_state).await?;

    Ok(())
}

async fn start_session(
    db_path: String,
    cli: CliArgs,
    ws_state: WsState,
) -> Result<(), Box<dyn std::error::Error>> {
    let events_tx = ws_state.events_tx.clone();
    let mut control_rx = ws_state.control_tx.subscribe();

    let backend: Arc<dyn Backend> = Arc::new(SqliteStore::new(&db_path).await?);

    let mut builder = Bot::builder()
        .with_backend(backend)
        .with_transport_factory(TokioWebSocketTransportFactory::new())
        .with_http_client(UreqHttpClient::new())
        .with_runtime(TokioRuntime)
        .with_device_props(
            DevicePropsOverride::new()
                .with_os("Android")
                .with_platform_type(PlatformType::AndroidPhone),
        );

    if let Some(phone) = cli.pair {
        tracing::info!("Requesting pair code for: {phone}");
        builder = builder.with_pair_code(PairCodeOptions {
            phone_number: phone,
            show_push_notification: true,
            custom_code: None,
            platform_id: Some(CompanionWebClientType::Chrome),
        });
    }

    let show_qr = cli.qrcode;
    let session = cli.session.clone();

    let mut bot = builder
        .on_event(move |event, _client| {
            let tx = events_tx.clone();
            let sname = session.clone();
            async move {
                let msg: Option<EventMessage> = match &*event {
                    Event::PairingQrCode { code, .. } => {
                        if show_qr {
                            tracing::info!("[{sname}] QR:\n{code}");
                        }
                        Some(EventMessage::event(
                            EventType::PairQr,
                            Payload::PairQr { code: code.clone() },
                        ))
                    }
                    Event::PairingCode { code, timeout } => {
                        tracing::info!(
                            "[{sname}] Pair code: {code} (expires in {}s)",
                            timeout.as_secs()
                        );
                        println!("[{sname}] Enter this code on your phone: {code}");
                        Some(EventMessage::event(
                            EventType::PairCode,
                            Payload::PairCode {
                                code: code.clone(),
                                expires_in: timeout.as_secs(),
                            },
                        ))
                    }
                    Event::PairSuccess(_) => {
                        tracing::info!("[{sname}] Paired successfully!");
                        Some(EventMessage::event(
                            EventType::PairSuccess,
                            Payload::Empty {},
                        ))
                    }
                    Event::PairError(_) => {
                        tracing::warn!("[{sname}] Pairing failed.");
                        Some(EventMessage::event(
                            EventType::PairError,
                            Payload::PairError {
                                reason: "Pairing failed".to_string(),
                            },
                        ))
                    }
                    Event::LoggedOut(_) => {
                        tracing::warn!("[{sname}] Logged out.");
                        Some(EventMessage::event(EventType::LoggedOut, Payload::Empty {}))
                    }
                    Event::Disconnected(_) => {
                        tracing::info!("[{sname}] Disconnected.");
                        Some(EventMessage::event(
                            EventType::Disconnected,
                            Payload::Empty {},
                        ))
                    }
                    _ => {
                        tracing::debug!("[{sname}] Event: {:?}", event);
                        None
                    }
                };

                if let Some(event_msg) = msg
                    && let Ok(json) = serde_json::to_string(&event_msg)
                {
                    let _ = tx.send(json);
                }
            }
        })
        .build()
        .await?;

    let client = bot.client();

    // Spawn control message handler
    tokio::spawn(async move {
        while let Ok(raw) = control_rx.recv().await {
            if let Ok(ctrl) = serde_json::from_str::<ControlMessage>(&raw) {
                match ctrl.kind {
                    ControlType::SendMessage => {
                        if let Payload::SendMessage { to, text } = ctrl.payload {
                            // tracing::info!("Sending message to {to}: {text}");
                            //  client.send_message(to.parse(), messages::Ex).await
                        }
                    }
                    ControlType::GetStatus => {
                        tracing::info!("Status requested");
                    }
                    ControlType::Disconnect => {
                        tracing::info!("Disconnect requested");
                        client.disconnect().await
                    }
                    ControlType::Logout => {
                        tracing::info!("Logout requested");
                        client.logout().await;
                    }
                }
            }
        }
    });

    bot.client()
        .set_client_profile(ClientProfile::android("13"))
        .await;

    bot.run().await?.await?;
    Ok(())
}
