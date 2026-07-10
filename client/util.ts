import P from "pino";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { promisify } from "node:util";
import { exec } from "node:child_process";
import { writeFileSync, unlinkSync, readFileSync } from "node:fs";
import { parse } from "smol-toml";
import { getAudioDuration } from "../lib";

interface BotConfig {
  metadata: {
    author: string;
    primary_platform: string;
    supported_platform: string;
  };
  auth: {
    strategy: "pairing_code" | "qr";
    qr_timeout_ms: number;
  };
  connection: {
    reconnect_on_failure: boolean;
    max_retries: number;
    proxy_url: string;
  };
  media: {
    auto_download: boolean;
  };
  features: {
    enable_logs: boolean;
    bot_name: string;
  };
}

const execPromise = promisify(exec);

const rawConfig = readFileSync("./config.toml", "utf-8");
export const config = parse(rawConfig) as unknown as BotConfig;

export const logger = P({
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

interface MediaOutput {
  buffer: Buffer;
  mimetype: string;
  seconds?: number;
}

async function processMedia(
  input: string | Buffer,
  outputExt: string,
  mimetype: string,
  ffmpegArgs: string,
): Promise<MediaOutput> {
  const tempInput = join(tmpdir(), `input_${Date.now()}`);
  const tempOutput = join(tmpdir(), `output_${Date.now()}.${outputExt}`);

  try {
    if (Buffer.isBuffer(input)) {
      writeFileSync(tempInput, input);
    } else {
      // If input is a path string, we can just use it, but tempInput simplifies the try/finally
      writeFileSync(tempInput, readFileSync(input));
    }

    await execPromise(
      `ffmpeg -i "${tempInput}" ${ffmpegArgs} -y "${tempOutput}"`,
    );

    const buffer = readFileSync(tempOutput);
    const seconds = await getAudioDuration(tempOutput);

    return {
      buffer,
      mimetype,
      seconds,
    };
  } finally {
    try {
      unlinkSync(tempInput);
    } catch {}
    try {
      unlinkSync(tempOutput);
    } catch {}
  }
}

export async function toMp3(input: string | Buffer): Promise<MediaOutput> {
  return processMedia(input, "mp3", "audio/mpeg", "-vn -ab 192k -ar 44100");
}

export async function toPTT(
  input: string | Buffer,
): Promise<MediaOutput & { ptt: true }> {
  const result = await processMedia(
    input,
    "ogg",
    "audio/ogg; codecs=opus",
    "-vn -c:a libopus -b:a 128k -vbr on",
  );

  return {
    ptt: true,
    ...result,
  };
}

export async function toMp4(input: string | Buffer): Promise<MediaOutput> {
  const tempInput = join(tmpdir(), `input_${Date.now()}`);
  const tempOutput = join(tmpdir(), `output_${Date.now()}.mp4`);

  try {
    if (Buffer.isBuffer(input)) {
      writeFileSync(tempInput, input);
    } else {
      writeFileSync(tempInput, readFileSync(input));
    }

    await execPromise(
      `ffmpeg -i "${tempInput}" -c:v libx264 -preset fast -crf 23 -c:a aac -b:a 128k -pix_fmt yuv420p -y "${tempOutput}"`,
    );

    const buffer = readFileSync(tempOutput);

    return {
      buffer,
      mimetype: "video/mp4",
    };
  } finally {
    try {
      unlinkSync(tempInput);
    } catch {}
    try {
      unlinkSync(tempOutput);
    } catch {}
  }
}
