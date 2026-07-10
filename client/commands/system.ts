import os from "node:os";
import { registerCommand } from "../plugin";
import { runtime, formatBytes } from "./menu";

registerCommand([
  {
    pattern: "ping",
    alias: ["p"],
    async func(msg) {
      const { client, remoteJid, id } = msg;
      const start = Date.now();
      const sent = await client.sendMessage(remoteJid, { text: `Ping ${id}` });
      const elapsed = Date.now() - start;
      await client.sendMessage(remoteJid, {
        edit: sent!.key,
        text: `\`\`\`Pong ${elapsed} ms\`\`\``,
      });
    },
  },
  {
    pattern: "uptime",
    alias: ["runtime"],
    async func(msg) {
      const uptime = runtime(process.uptime());
      return msg.client.sendMessage(msg.remoteJid, {
        text: `\`\`\`Running since: ${uptime}\`\`\``,
      });
    },
  },
  {
    pattern: "ram",
    alias: ["memory"],
    async func(msg) {
      const total = os.totalmem();
      const used = total - os.freemem();
      const pct = ((used / total) * 100).toFixed(1);
      return msg.client.sendMessage(msg.remoteJid, {
        text: `\`\`\`RAM: ${formatBytes(used)} / ${formatBytes(total)} (${pct}%)\`\`\``,
      });
    },
  },
  {
    pattern: "sysinfo",
    async func(msg) {
      const total = os.totalmem();
      const used = total - os.freemem();
      const cpus = os.cpus();
      const load = os.loadavg();

      const text =
        `\`\`\`System Info\n` +
        `Platform : ${os.platform()} ${os.arch()}\n` +
        `Hostname : ${os.hostname()}\n` +
        `CPU      : ${cpus[0]?.model ?? "Unknown"} (${cpus.length} cores)\n` +
        `Load avg : ${load.map((l) => l.toFixed(2)).join(" | ")}\n` +
        `RAM      : ${formatBytes(used)} / ${formatBytes(total)}\n` +
        `Uptime   : ${runtime(process.uptime())}\`\`\``;

      return msg.client.sendMessage(msg.remoteJid, { text });
    },
  },
  {
    pattern: "alive",
    dontAddToCommandList: true,
    async func(msg) {
      const actl = ["🎶", "💯", "🤖"];

      return msg.client.sendMessage(msg.remoteJid, {
        react: {
          key: msg,
          text: actl[Math.floor(Math.random() * actl.length)],
        },
      });
    },
  },
]);
