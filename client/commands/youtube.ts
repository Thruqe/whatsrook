import { exec } from "node:child_process";
import { promisify } from "node:util";
import axios from "axios";

import { registerCommand } from "../plugin";
import { toMp4 } from "../util";

const execPromise = promisify(exec);
const downloadState = new Map<
  string,
  { url: string; formats: any[]; title: string }
>();

async function downloadAndSend(
  msg: any,
  url: string,
  formatId: string,
  title: string,
) {
  try {
    await msg.client.sendMessage(msg.remoteJid, {
      text: `Processing *${title}*...`,
    });

    const { stdout: thumbUrl } = await execPromise(
      `yt-dlp --get-thumbnail "${url}"`,
    );
    const thumbResponse = await axios.get(thumbUrl.toString().trim(), {
      responseType: "arraybuffer",
    });
    const jpegThumbnail = Buffer.from(thumbResponse.data).toString("base64");

    const command = `yt-dlp "${url}" -f "${formatId}" -o -`;
    const { stdout } = await execPromise(command, {
      encoding: null,
      maxBuffer: 1024 * 1024 * 100,
    });

    const videoData = await toMp4(stdout);

    await msg.client.sendMessage(msg.remoteJid, {
      video: videoData.buffer,
      mimetype: "video/mp4",
      caption: title,
      jpegThumbnail,
    });
  } catch (e) {
    await msg.client.sendMessage(msg.remoteJid, { text: "Download failed." });
  }
}

registerCommand([
  {
    pattern: "ytv",
    category: "downloader",
    async func(msg) {
      const url = msg.args[1];
      if (!url)
        return await msg.client.sendMessage(msg.remoteJid, {
          text: "Provide a YouTube URL",
        });

      try {
        const { stdout } = await execPromise(`yt-dlp "${url}" -J`);
        const metadata = JSON.parse(stdout.toString());

        let formats = metadata.formats.filter(
          (f: any) =>
            f.height <= 480 && f.vcodec !== "none" && f.acodec !== "none",
        );

        if (formats.length === 0) {
          formats = [
            {
              format_id: "bestvideo[height<=480]+bestaudio/best[height<=480]",
              height: 480,
              filesize: 0,
            },
          ];
        }

        if (formats.length === 1) {
          return await downloadAndSend(
            msg,
            url,
            formats[0].format_id,
            metadata.title,
          );
        }

        downloadState.set(msg.remoteJid, {
          url,
          formats,
          title: metadata.title,
        });

        let menu = `*${metadata.title}*\n\nReply with a number:\n\n`;
        formats.forEach((f: any, i: number) => {
          const size = f.filesize || f.filesize_approx || 0;
          const sizeMB = size > 0 ? (size / 1024 / 1024).toFixed(1) : "Unknown";
          menu += `*${i + 1}* - ${f.height}p (~${sizeMB}MB)\n`;
        });

        await msg.client.sendMessage(msg.remoteJid, { text: menu });
      } catch (error) {
        await msg.client.sendMessage(msg.remoteJid, {
          text: "Error fetching video info.",
        });
      }
    },
  },
  {
    event: true,
    async func(msg) {
      if (!downloadState.has(msg.remoteJid)) return;

      const body = msg.body.trim();
      const choice = parseInt(body);
      if (isNaN(choice)) return;

      const state = downloadState.get(msg.remoteJid)!;
      const index = choice - 1;

      if (index >= 0 && index < state.formats.length) {
        const selected = state.formats[index];
        downloadState.delete(msg.remoteJid);
        await downloadAndSend(msg, state.url, selected.format_id, state.title);
      }
    },
  },
]);
