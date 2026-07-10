import type { SocketContext } from "./types.ts";

export type OnWhatsAppResult = {
	exists: boolean;
	jid: string;
	lid?: string;
	pnJid?: string;
	isBusiness?: boolean;
};

export const makeContactMethods = (ctx: SocketContext) => ({
	/**
	 * Checks if the provided identifiers are registered on WhatsApp.
	 */
	onWhatsApp: async (...jids: string[]): Promise<OnWhatsAppResult[]> => {
		const client = await ctx.getClient();
		const results = await client.isOnWhatsApp(jids);
		return results.map(r => {
			const out: OnWhatsAppResult = {
				exists: r.isRegistered,
				jid: r.jid,
				isBusiness: r.isBusiness
			};
			if (r.lid) out.lid = r.lid;
			if (r.pnJid) out.pnJid = r.pnJid;
			return out;
		});
	},

	/**
	 * Retrieves the profile picture URL for a specific JID.
	 */
	profilePictureUrl: async (
		jid: string,
		type: "preview" | "image" = "preview"
	) => {
		const result = await (await ctx.getClient()).profilePictureUrl(jid, type);
		return result?.url;
	},

	/**
	 * Fetches detailed user information for the specified identifiers.
	 * Note: Some details may not be available due to the User's Privacy Settings.
	 */
	fetchUserInfo: async (...jids: string[]) => {
		return await (await ctx.getClient()).fetchUserInfo(jids);
	}
});
