import { join } from "node:path";
import { writeFile, mkdir } from "node:fs/promises";
import { fileTypeFromBuffer } from "file-type";
import { config, isMediaMessage } from "./util";
import type { WAMessage, WASocket } from "../lib";

export const autoDownload = async (message: WAMessage, sock: WASocket) => {
	if (config?.media?.auto_download && isMediaMessage(message)) {
		try {
			const tmpDir = join(process.cwd(), "tmp");
			await mkdir(tmpDir, { recursive: true });

			const buffer = await sock.downloadMedia(message, "buffer");
			if (!buffer) return;

			const type = await fileTypeFromBuffer(buffer);
			const extension = type ? `.${type.ext}` : "";

			const fileName = `${message.key.id || Date.now()}${extension}`;
			const filePath = join(tmpDir, fileName);

			await writeFile(filePath, buffer);

			return filePath;
		} catch {
			return;
		}
	}
};
