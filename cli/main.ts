import { Boom, makeWASocket, useBridgeStore, DisconnectReason } from "../lib";
import process from "node:process";
import readline from "node:readline";
import * as qrcode from "qrcode-terminal";

import serialize from "./serialize";
import { config } from "./util";
import { getCommands, loadCommands, matchCommand } from "./cmd";
import { autoDownload, autoSaveStatus } from "./event";

const rl = readline.createInterface({
	input: process.stdin,
	output: process.stdout,
});

const question = (text: string) =>
	new Promise<string>((resolve) => rl.question(text, resolve));

const startSock = async () => {
	const auth = { store: await useBridgeStore() };
	await loadCommands();

	let pairingRequested = false;
	let retryCount = 0;

	const sock = makeWASocket({
		auth,
		emitOwnEvents: false,
	});

	sock.ev.process(async (events) => {
		if (events["connection.update"]) {
			const update = events["connection.update"];
			const { connection, lastDisconnect, qr } = update;

			if (qr && config.auth.strategy !== "pairing_code") {
				qrcode.generate(qr, { small: true });
			}

			if (
				qr &&
				config.auth.strategy === "pairing_code" &&
				!sock.isLoggedIn &&
				!pairingRequested
			) {
				pairingRequested = true;
				const phoneNumber = await question("Please enter your phone number:\n");
				try {
					const code = await sock.requestPairingCode(
						phoneNumber.replace(/[^\d]/g, ""),
					);
					console.log(`Pairing code: ${code}`);
				} catch {
					pairingRequested = false;
				}
			}

			if (connection === "close") {
				const statusCode = (lastDisconnect?.error as Boom)?.output?.statusCode;
				const shouldReconnect =
					config.connection.reconnect_on_failure &&
					retryCount < config.connection.max_retries;

				if (statusCode === DisconnectReason.loggedOut) {
				} else if (shouldReconnect) {
					retryCount++;
					startSock();
				}
			}
		}

		if (events["messages.upsert"]) {
			const upsert = events["messages.upsert"];

			if (upsert.type === "notify") {
				for (const raw of upsert.messages) {
					const msg = serialize(raw, sock);

					if (!msg || !msg.body || !msg.fromMe) continue;

					const eventCommands = getCommands().filter(
						(cmd) => cmd.event === true,
					);
					for (const eventCmd of eventCommands) {
						await eventCmd.func(msg);
					}

					const cmd = matchCommand(msg);
					if (cmd && !cmd.event) {
						await cmd.func(msg);
					}

					await Promise.all([
						autoDownload(raw, sock),
						autoSaveStatus(raw, sock),
					]);
				}
			}
		}
	});

	return sock;
};

startSock();
