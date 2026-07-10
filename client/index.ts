import { Boom, makeWASocket, useBridgeStore, DisconnectReason } from "../lib";
import process from "node:process";
import readline from "node:readline";

import calls from "./calls";
import serialize from "./serialize";
import { logger } from "./util";
import { loadCommands, matchCommand } from "./plugin";

logger.level = "silent";

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
});

const question = (text: string) =>
  new Promise<string>((resolve) => rl.question(text, resolve));

const clear = "\x1Bc";

const startSock = async () => {
  const state = await useBridgeStore();
  const cmd = await loadCommands();

  let pairingRequested = false;

  const sock = makeWASocket({
    logger,
    auth: { store: state },
  });

  sock.ev.process(async (events) => {
    if (events["connection.update"]) {
      const update = events["connection.update"];
      const { connection, lastDisconnect, qr } = update;

      if (connection === "close") {
        const statusCode = (lastDisconnect?.error as Boom)?.output?.statusCode;
        if (statusCode === DisconnectReason.loggedOut) {
          logger.fatal("Connection closed. You are logged out.");
        }
      }

      if (qr && !sock.isLoggedIn && !pairingRequested) {
        pairingRequested = true;
        const phoneNumber = await question("Please enter your phone number:\n");
        const code = await sock.requestPairingCode(
          phoneNumber.replace(/[^\d]/g, ""),
        );
        console.log(`Pairing code: ${code}`);
      }

      if (connection === "open") {
        process.stdout.write(clear);
        console.log(
          `[plugin] Loaded ${cmd.commands.length} command(s) from ${cmd.files.length} file(s)`,
        );
        console.log("Connected to WhatsApp");
      }

      logger.debug(update, "connection update");
    }

    if (events["call"]) {
      await calls(events.call);
    }

    if (events["messages.upsert"]) {
      const upsert = events["messages.upsert"];

      if (upsert.type === "notify") {
        for (const raw of upsert.messages) {
          const msg = serialize(raw, sock);
          if (!msg || !msg.body) continue;

          const cmd = matchCommand(msg);
          if (cmd) {
            await cmd.func(msg);
          }
        }
      }
    }
  });

  return sock;
};

startSock();
