import {
	Boom,
	makeWASocket,
	createLogger,
	useBridgeStore,
	DisconnectReason,
} from "zevbot";
import * as qrcode from "qrcode-terminal";

const logger = createLogger("trace");

const server = Bun.serve({
	port: process.env.PORT || 3000,
	fetch(req, server) {
		const url = new URL(req.url);
		if (url.pathname === "/ws") {
			const success = server.upgrade(req);
			if (success) return undefined;
			return new Response("WebSocket upgrade failed", { status: 400 });
		}
		return new Response("Not Found", { status: 404 });
	},
	websocket: {
		open(ws) {
			ws.subscribe("whatsapp-events");
			logger.info(`WebSocket client connected: ${ws.remoteAddress}`);
		},
		message(ws, message) {
			server.publish("whatsapp-control", message);
		},
		close(ws) {
			ws.unsubscribe("whatsapp-events");
			logger.info(`WebSocket client disconnected: ${ws.remoteAddress}`);
		},
	},
});

const startSock = async () => {
	const auth = { store: await useBridgeStore() };

	const sock = makeWASocket({
		auth,
		logger,
		emitOwnEvents: false,
	});

	sock.ev.process(async (events) => {
		server.publish(
			"whatsapp-events",
			JSON.stringify(events, (_, value) =>
				typeof value === "bigint" ? value.toString() : value,
			),
		);

		if (events["connection.update"]) {
			const update = events["connection.update"];
			const { connection, lastDisconnect, qr } = update;

			if (qr) {
				qrcode.generate(qr, { small: true });
			}

			if (connection === "close") {
				const statusCode = (lastDisconnect?.error as Boom)?.output?.statusCode;

				if (statusCode === DisconnectReason.loggedOut) {
					logger.info("Connection Logged Out.");
				} else {
					logger.info("Connection closed.");
				}
			}
		}
	});

	return sock;
};

startSock();
