import type { SocketContext } from "./types.mts";

export const makeBlockingMethods = (ctx: SocketContext) => ({
	updateBlockStatus: async (jid: string, action: "block" | "unblock") => {
		await (await ctx.getClient()).updateBlockStatus(jid, action);
	},

	fetchBlocklist: async () => {
		return await (await ctx.getClient()).fetchBlocklist();
	}
});
