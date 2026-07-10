mod cli;
mod messages;
mod utils;
mod ws;

use axum::{Router, routing::get};
use std::env;
use whatsapp_rust::{
    ClientProfile, TokioRuntime,
    bot::Bot,
    http::UreqHttpClient,
    pair::CompanionWebClientType,
    send::RevokeType,
    store::SqliteStore,
    transport::TokioWebSocketTransportFactory,
    wacore::{
        pair_code::PairCodeOptions,
        proto_helpers::{MessageExt, build_quote_context},
        store::DevicePropsOverride,
        types::events::Event,
    },
    waproto::whatsapp::{self as wa, device_props::PlatformType},
};

use cli::{CliArgs, ClientType};
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

    let backend = SqliteStore::new(&db_path).await?;

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
        let platform_id = match cli.client {
            ClientType::Chrome => Some(CompanionWebClientType::Chrome),
            ClientType::Android => None, // Android pairs via QR, not companion web
            ClientType::Ios => None,
        };

        builder = builder.with_pair_code(PairCodeOptions {
            phone_number: phone,
            show_push_notification: true,
            custom_code: None,
            platform_id,
        });
    }

    let show_qr = cli.qrcode;
    let session = cli.session.clone();

    let bot = builder
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
                    Event::Message(wa_msg, info) => {
                        let text = wa_msg
                            .text_content()
                            .map(|s| s.to_string())
                            .unwrap_or_default();
                        let from = info.source.sender.to_string();
                        let message_id = info.id.clone();
                        tracing::info!("[{sname}] Message from {from}: {text}");
                        Some(EventMessage::event(
                            EventType::Message,
                            Payload::IncomingMessage {
                                from,
                                text,
                                message_id,
                            },
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

    tokio::spawn(async move {
        while let Ok(raw) = control_rx.recv().await {
            let ctrl = match serde_json::from_str::<ControlMessage>(&raw) {
                Ok(c) => c,
                Err(e) => {
                    tracing::warn!("Bad control message: {e}");
                    continue;
                }
            };

            match ctrl.kind {
                ControlType::SendMessage => {
                    if let Payload::SendMessage {
                        to,
                        text,
                        quote_id,
                        quote_sender,
                    } = ctrl.payload
                    {
                        let to_jid: whatsapp_rust::Jid = match to.parse() {
                            Ok(j) => j,
                            Err(e) => {
                                tracing::warn!("Invalid JID '{to}': {e}");
                                continue;
                            }
                        };
                        let message = if let (Some(qid), Some(qsender)) = (quote_id, quote_sender) {
                            let context =
                                build_quote_context(&qid, &qsender, &wa::Message::default());
                            wa::Message {
                                extended_text_message: Some(Box::new(
                                    wa::message::ExtendedTextMessage {
                                        text: Some(text),
                                        context_info: Some(Box::new(context)),
                                        ..Default::default()
                                    },
                                )),
                                ..Default::default()
                            }
                        } else {
                            wa::Message {
                                conversation: Some(text),
                                ..Default::default()
                            }
                        };
                        match client.send_message(to_jid, message).await {
                            Ok(r) => tracing::info!("Sent: {}", r.message_id),
                            Err(e) => tracing::error!("Send failed: {e}"),
                        }
                    }
                }

                ControlType::SendReaction => {
                    if let Payload::SendReaction {
                        to,
                        message_id,
                        sender,
                        emoji,
                    } = ctrl.payload
                    {
                        let to_jid: whatsapp_rust::Jid = match to.parse() {
                            Ok(j) => j,
                            Err(e) => {
                                tracing::warn!("Invalid JID '{to}': {e}");
                                continue;
                            }
                        };

                        let target_key = wa::MessageKey {
                            remote_jid: Some(to.clone()),
                            from_me: None,
                            id: Some(message_id.clone()),
                            participant: sender.clone(),
                        };
                        match client.send_reaction(&to_jid, target_key, &emoji).await {
                            Ok(_) => tracing::info!("Reaction '{emoji}' sent"),
                            Err(e) => tracing::error!("Reaction failed: {e}"),
                        }
                    }
                }

                ControlType::EditMessage => {
                    if let Payload::EditMessage {
                        to,
                        message_id,
                        new_text,
                    } = ctrl.payload
                    {
                        let to_jid: whatsapp_rust::Jid = match to.parse() {
                            Ok(j) => j,
                            Err(e) => {
                                tracing::warn!("Invalid JID '{to}': {e}");
                                continue;
                            }
                        };
                        let new_content = wa::Message {
                            conversation: Some(new_text),
                            ..Default::default()
                        };
                        match client.edit_message(to_jid, &message_id, new_content).await {
                            Ok(id) => tracing::info!("Edited, new id: {id}"),
                            Err(e) => tracing::error!("Edit failed: {e}"),
                        }
                    }
                }

                ControlType::RevokeMessage => {
                    if let Payload::RevokeMessage {
                        to,
                        message_id,
                        original_sender,
                    } = ctrl.payload
                    {
                        let to_jid: whatsapp_rust::Jid = match to.parse() {
                            Ok(j) => j,
                            Err(e) => {
                                tracing::warn!("Invalid JID '{to}': {e}");
                                continue;
                            }
                        };
                        let revoke_type = match original_sender {
                            Some(s) => match s.parse() {
                                Ok(jid) => RevokeType::Admin {
                                    original_sender: jid,
                                },
                                Err(e) => {
                                    tracing::warn!("Bad original_sender: {e}");
                                    continue;
                                }
                            },
                            None => RevokeType::Sender,
                        };
                        match client
                            .revoke_message(to_jid, &message_id, revoke_type)
                            .await
                        {
                            Ok(_) => tracing::info!("Revoked {message_id}"),
                            Err(e) => tracing::error!("Revoke failed: {e}"),
                        }
                    }
                }

                ControlType::GetStatus => {
                    tracing::info!(
                        "Status — connected: {}, logged_in: {}",
                        client.is_connected(),
                        client.is_logged_in()
                    );
                }
                ControlType::Disconnect => {
                    tracing::info!("Disconnect requested");
                    client.disconnect().await;
                }
                ControlType::Logout => {
                    tracing::info!("Logout requested");
                    let _ = client.logout().await;
                }
            }
        }
    });

    let profile = match cli.client {
        ClientType::Chrome => ClientProfile::web(),
        ClientType::Android => ClientProfile::android("16"),
        ClientType::Ios => ClientProfile::ios("17"),
    };
    bot.client().set_client_profile(profile).await;
    Ok(())
}
