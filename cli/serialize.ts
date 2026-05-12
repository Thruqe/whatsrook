import { isJidGroup, type WAMessage, type WASocket } from "../lib";
import { extractText } from "./util";

export type Client = WASocket;

export interface SerializedMessage {
	raw: WAMessage;

	client: Client;

	remoteJid: string;

	id: string;

	fromMe: boolean;

	isGroup: boolean;

	sender: string;

	body: string;

	timestamp: number;

	args: string[];

	command: string;
}

export default function (
	msg: WAMessage,
	client: Client
): SerializedMessage | null {
	const remoteJid = msg.key.remoteJid;
	if (!remoteJid) return null;

	const isGroup = isJidGroup(remoteJid)!;
	const fromMe = msg.key.fromMe!;

	// In groups the participant field holds the actual sender
	const sender = isGroup
		? (msg.key.participant ?? msg.participant ?? remoteJid)
		: fromMe
			? (client.user?.id ?? remoteJid)
			: remoteJid;

	const body = extractText(msg);

	const args = body.trim().split(/\s+/).filter(Boolean);
	const command = (args[0] ?? "").toLowerCase();

	return {
		raw: msg,
		client,
		remoteJid,
		id: msg.key.id ?? "",
		fromMe,
		isGroup,
		sender,
		body,
		timestamp:
			typeof msg.messageTimestamp === "number"
				? msg.messageTimestamp
				: Number(msg.messageTimestamp ?? 0),
		args,
		command
	};
}
