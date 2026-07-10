import readline from "node:readline";
import {
  Boom,
  Browsers,
  makeWASocket,
  useBridgeStore,
  DisconnectReason,
  fetchLatestWaWebVersion,
} from "../lib";
import P from "pino";
import process from "node:process";

const logger = P({
  level: "trace",
  transport: {
    targets: [
      {
        target: "pino-pretty",
        options: { colorize: true },
        level: "trace",
      },
      {
        target: "pino/file",
        options: { destination: "./logs.txt" },
        level: "trace",
      },
    ],
  },
});

logger.level = "trace";

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
});

const question = (text: string) =>
  new Promise<string>((resolve) => rl.question(text, resolve));

const startSock = async () => {
  const latest = await fetchLatestWaWebVersion();
  const { version } = latest;
  const state = await useBridgeStore("auth");
  logger.debug(
    { version: version.join("."), isLatest: latest.isLatest },
    `using latest WA version`,
  );

  const sock = makeWASocket({
    logger,
    version,
    auth: { store: state },
    browser: Browsers.android("16"),
    deviceProps: {
      platformType: "ANDROID_PHONE",
      os: "Android",
    },
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

      if (qr && !sock.isLoggedIn) {
        const phoneNumber = await question("Please enter your phone number:\n");
        const code = await sock.requestPairingCode(phoneNumber);
        console.log(`Pairing code: ${code}`);
      }

      logger.debug(update, "connection update");
    }

    if (events["call"]) {
      logger.debug(events["call"], "call event fired");
    }

    if (events["messages.upsert"]) {
      const upsert = events["messages.upsert"];
      logger.debug(upsert, "messages.upsert fired");

      if (upsert.requestId) {
        logger.debug(upsert, "placeholder request message received");
      }

      if (upsert.type === "notify") {
        for (const msg of upsert.messages) {
          if (
            msg.message?.conversation ||
            msg.message?.extendedTextMessage?.text
          ) {
            const text = (
              msg.message?.conversation ||
              msg.message?.extendedTextMessage?.text
            )
              ?.toLowerCase()
              .trim();
            if (text == "onDemandHistSync") {
              const messageId = await sock.fetchMessageHistory(
                50,
                msg.key,
                msg.messageTimestamp!,
              );
              logger.debug(
                { id: messageId },
                "requested on-demand history resync",
              );
            }

            if (text === "ping") {
              const start = Date.now();
              const i = await sock.sendMessage(msg.key.remoteJid!, {
                text: "Ping " + msg.key.id,
              });
              const end = Date.now();
              await sock.sendMessage(msg.key.remoteJid!, {
                edit: i?.key!,
                text: `\`\`\`Pong ${end - start} ms\`\`\``,
              });
            }
          }
        }
      }
    }

    if (events["messages.update"]) {
      logger.debug(events["messages.update"], "messages.update fired");

      for (const { update } of events["messages.update"]) {
        if (update.pollUpdates) {
          logger.debug({ pollUpdates: update.pollUpdates }, "got poll update");
        }
      }
    }
  });

  return sock;
};

startSock();
