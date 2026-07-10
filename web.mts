import {
	Boom,
	makeWASocket,
	createLogger,
	useBridgeStore,
	DisconnectReason
} from "zevbot";
import { join } from "path";

const logger = createLogger("trace");

const server = Bun.serve({
	port: process.env.PORT || 3000,
	async fetch(req, server) {
		const url = new URL(req.url);

		if (url.pathname === "/ws") {
			const success = server.upgrade(req);
			if (success) return undefined;
			return new Response("WebSocket upgrade failed", { status: 400 });
		}

		let filePath = join(import.meta.dir, "web", url.pathname);
		if (url.pathname.endsWith("/")) {
			filePath = join(filePath, "index.html");
		}

		const file = Bun.file(filePath);
		if (await file.exists()) {
			return new Response(file);
		}

		const fallbackIndex = Bun.file(join(import.meta.dir, "web", "index.html"));
		if (await fallbackIndex.exists()) {
			return new Response(fallbackIndex);
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
		}
	}
});

const getSessionName = (): string | undefined => {
	const index = process.argv.indexOf("--session");
	if (index !== -1 && index + 1 < process.argv.length) {
		return process.argv[index + 1];
	}
	return undefined;
};

const startSock = async () => {
	const sessionName = getSessionName();
	const auth = { store: await useBridgeStore(sessionName) };
	const emitOwnEvents = false;

	const sock = makeWASocket({
		auth,
		logger,
		emitOwnEvents
	});

	sock.ev.process(async events => {
		server.publish(
			"whatsapp-events",
			JSON.stringify(events, (_, value) =>
				typeof value === "bigint" ? value.toString() : value
			)
		);

		if (events["connection.update"]) {
			const update = events["connection.update"];
			const { connection, lastDisconnect, qr } = update;

			if (qr) {
				server.publish("CONNECTION_UPDATE", qr);
			}

			if (connection === "close") {
				const statusCode = (lastDisconnect?.error as Boom)?.output?.statusCode;

				if (statusCode === DisconnectReason.loggedOut) {
					server.publish("CONNECTION_UPDATE", "Connection Logged Out.");
				} else {
					server.publish("CONNECTION_UPDATE", "Connection closed.");
				}
			}
		}
	});

	return sock;
};

startSock();
