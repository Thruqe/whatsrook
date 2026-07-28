// Bun server: serves the dashboard UI and bridges browser WebSocket
// clients to the WhatsRook daemon's binary Protobuf WebSocket.

import { EventFrame, ControlFrame, ControlType } from "./proto/wsproto/ws";

const DAEMON_URL = process.env.WHATSROOK_WS_URL ?? "ws://localhost:8080/ws";
const PREFERRED_PORT = Number(process.env.PORT ?? 3000);

// --- Connection to the WhatsRook daemon -----------------------------------

let daemonSocket: WebSocket | null = null;
const browserSockets = new Set<import("bun").ServerWebSocket<unknown>>();

function connectToDaemon() {
    console.log(`[daemon] connecting to ${DAEMON_URL}...`);
    const sock = new WebSocket(DAEMON_URL, ["protobuf"]);
    sock.binaryType = "arraybuffer";

    sock.addEventListener("open", () => {
        console.log("[daemon] connected");
        sendControl({ type: ControlType.CONTROL_TYPE_GET_STATUS, id: reqId("status"), payload: { $case: "getStatus", getStatus: {} } });
    });

    sock.addEventListener("message", (event: MessageEvent) => {
        if (!(event.data instanceof ArrayBuffer)) return;
        let frame: EventFrame;
        try {
            frame = EventFrame.decode(new Uint8Array(event.data));
        } catch (err) {
            console.error("[daemon] failed to decode EventFrame:", err);
            return;
        }
        broadcastToBrowsers(frameToBrowserEvent(frame));
    });

    sock.addEventListener("close", () => {
        console.log("[daemon] disconnected, retrying in 3s...");
        daemonSocket = null;
        setTimeout(connectToDaemon, 3000);
    });

    sock.addEventListener("error", (err) => {
        console.error("[daemon] socket error:", err);
    });

    daemonSocket = sock;
}

function reqId(prefix: string): string {
    return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function sendControl(frame: ControlFrame) {
    if (!daemonSocket || daemonSocket.readyState !== WebSocket.OPEN) {
        console.warn("[daemon] cannot send, not connected");
        return;
    }
    daemonSocket.send(ControlFrame.encode(frame).finish());
}

// Translate a daemon EventFrame into a friendly JSON event for the browser.
function frameToBrowserEvent(frame: EventFrame): { type: string; data: unknown } {
    switch (frame.payload?.$case) {
        case "status": {
            const s = frame.payload.status;
            return { type: "status", data: { connected: s.connected, loggedIn: s.loggedIn, jid: s.jid ?? null, pushName: s.pushName ?? null } };
        }
        case "pairQr":
            return { type: "pair_qr", data: { code: frame.payload.pairQr.code } };
        case "pairCode":
            return { type: "pair_code", data: { code: frame.payload.pairCode.code } };
        case "pairError":
            return { type: "pair_error", data: { reason: frame.payload.pairError.reason } };
        case "ack":
            return { type: "ack", data: { ok: frame.payload.ack.ok, error: frame.payload.ack.error ?? null } };
        case "message": {
            const m = frame.payload.message;
            return {
                type: "message",
                data: {
                    from: m.from,
                    chat: m.chat,
                    sender: m.sender,
                    text: m.text,
                    messageId: m.messageId,
                    pushName: m.pushName ?? null,
                    timestamp: m.timestampUnix,
                    isGroup: m.isGroup,
                    isFromMe: m.isFromMe,
                },
            };
        }
        case "incomingCall":
            return { type: "incoming_call", data: { callId: frame.payload.incomingCall.callId, from: frame.payload.incomingCall.from } };
        default:
            return { type: "unknown", data: {} };
    }
}

function broadcastToBrowsers(event: { type: string; data: unknown }) {
    const payload = JSON.stringify(event);
    for (const ws of browserSockets) {
        ws.send(payload);
    }
}

// --- HTTP + browser WebSocket server ---------------------------------------

connectToDaemon();

function tryServe(port: number): ReturnType<typeof Bun.serve> {
    try {
        return Bun.serve({
            port,
            async fetch(req, server) {
                const url = new URL(req.url);

                if (url.pathname === "/ws") {
                    const upgraded = server.upgrade(req, { headers: undefined, data: null });
                    if (upgraded) return undefined;
                    return new Response("WebSocket upgrade failed", { status: 400 });
                }

                let filePath = url.pathname === "/" ? "/html/index.html" : url.pathname;
                if (filePath.startsWith("/js/") || filePath.startsWith("/html/") || filePath.startsWith("/css/")) {
                    const file = Bun.file(`.${filePath}`);
                    if (await file.exists()) return new Response(file);
                }
                return new Response("Not found", { status: 404 });
            },
            websocket: {
                open(ws) {
                    browserSockets.add(ws);
                    console.log("[browser] client connected");
                },
                close(ws) {
                    browserSockets.delete(ws);
                    console.log("[browser] client disconnected");
                },
                message(ws, message) {
                    // Browser sends simple JSON commands; translate to ControlFrame.
                    let msg: { action: string; payload?: Record<string, unknown> };
                    try {
                        msg = JSON.parse(String(message));
                    } catch {
                        return;
                    }

                    switch (msg.action) {
                        case "request_pair_code":
                            sendControl({
                                type: ControlType.CONTROL_TYPE_REQUEST_PAIR_CODE,
                                id: reqId("pair"),
                                payload: { $case: "requestPairCode", requestPairCode: { phoneNumber: String(msg.payload?.phoneNumber ?? "") } },
                            });
                            break;
                        case "request_pair_qr":
                            sendControl({
                                type: ControlType.CONTROL_TYPE_REQUEST_PAIR_QR,
                                id: reqId("qr"),
                                payload: { $case: "requestPairQr", requestPairQr: {} },
                            });
                            break;
                        case "get_status":
                            sendControl({
                                type: ControlType.CONTROL_TYPE_GET_STATUS,
                                id: reqId("status"),
                                payload: { $case: "getStatus", getStatus: {} },
                            });
                            break;
                        case "logout":
                            sendControl({
                                type: ControlType.CONTROL_TYPE_LOGOUT,
                                id: reqId("logout"),
                                payload: { $case: "logout", logout: {} },
                            });
                            break;
                        case "disconnect":
                            sendControl({
                                type: ControlType.CONTROL_TYPE_DISCONNECT,
                                id: reqId("disconnect"),
                                payload: { $case: "disconnect", disconnect: {} },
                            });
                            break;
                    }
                },
            },
        });
    } catch (err: any) {
        if (err?.code === "EADDRINUSE" || err?.message?.includes("address already in use")) {
            console.warn(`[server] Port ${port} is in use, trying ${port + 1}...`);
            return tryServe(port + 1);
        }
        throw err;
    }
}

const server = tryServe(PREFERRED_PORT);
console.log(`Dashboard server running at http://localhost:${server.port}`);