import {
	Boom,
	makeWASocket,
	createLogger,
	useBridgeStore,
	DisconnectReason,
} from "zevbot";
import * as qrcode from "qrcode-terminal";

const logger = createLogger("trace");

const startSock = async () => {
	const auth = { store: await useBridgeStore() };

	const sock = makeWASocket({
		auth,
		logger,
		emitOwnEvents: false,
	});

	sock.ev.process(async (events) => {
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
