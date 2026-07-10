import {
	Boom,
	makeWASocket,
	createLogger,
	useBridgeStore,
	DisconnectReason
} from "zevbot";

const logger = createLogger("trace");

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
		logger: undefined,
		emitOwnEvents
	});

	sock.ev.process(async events => {
		if (events["connection.update"]) {
			const update = events["connection.update"];
			const { connection, lastDisconnect, qr } = update;

			if (qr) {
				logger.info(qr);
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
