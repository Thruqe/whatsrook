import type { ChatModification } from "../Types/index.ts";
import type { SocketContext } from "./types.ts";

export const makeChatActionMethods = (ctx: SocketContext) => ({
	pinChat: async (jid: string, pin: boolean) => {
		await (await ctx.getClient()).pinChat(jid, pin);
	},

	muteChat: async (jid: string, muteUntil?: number | null) => {
		await (await ctx.getClient()).muteChat(jid, muteUntil);
	},

	archiveChat: async (jid: string, archive: boolean) => {
		await (await ctx.getClient()).archiveChat(jid, archive);
	},

	starMessage: async (jid: string, messageId: string, star: boolean) => {
		await (await ctx.getClient()).starMessage(jid, messageId, star);
	},

	/**
	 * Compatibility wrapper for original Baileys chatModify API.
	 * Routes to the appropriate bridge method based on the modification type.
	 *
	 * Fully supported: archive, pin, mute, star, markRead, delete, deleteForMe, pushNameSetting
	 * Not yet in bridge (app-state patches): clear, contact, disableLinkPreviews, labels, quickReply
	 */
	chatModify: async (mod: ChatModification, jid: string) => {
		const client = await ctx.getClient();
		if ("archive" in mod) {
			await client.archiveChat(jid, mod.archive);
		} else if ("pin" in mod) {
			await client.pinChat(jid, mod.pin);
		} else if ("mute" in mod) {
			await client.muteChat(jid, mod.mute);
		} else if ("star" in mod) {
			for (const msg of mod.star.messages) {
				await client.starMessage(jid, msg.id, mod.star.star);
			}
		} else if ("markRead" in mod) {
			await client.markChatAsRead(jid, mod.markRead);
		} else if ("delete" in mod) {
			await client.deleteChat(jid);
		} else if ("deleteForMe" in mod) {
			await client.deleteMessageForMe(
				jid,
				mod.deleteForMe.key.id!,
				!!mod.deleteForMe.key.fromMe
			);
		} else if ("pushNameSetting" in mod) {
			await client.setPushName(mod.pushNameSetting);
		} else {
			// App-state-patch variants not yet exposed by bridge:
			// clear, contact, disableLinkPreviews, addLabel, addChatLabel,
			// removeChatLabel, addMessageLabel, removeMessageLabel, quickReply
			const variant = Object.keys(mod)[0];
			ctx.logger?.warn(
				{ variant, jid },
				"chatModify: variant requires app-state patch support not yet available in bridge"
			);
		}
	}
});
