import type { WhatsAppEvent } from "whatsapp-rust-bridge";
import type { proto } from "whatsapp-rust-bridge/proto-types";
import type { ILogger } from "../Utils/logger.ts";
import { processHistoryMessage } from "../Utils/process-history-message.ts";
import type {
	CanonicalCallAction,
	CanonicalCallActionType,
	CanonicalEvent,
	CanonicalGroupAction,
	CanonicalGroupParticipant
} from "./types.ts";
import {
	asBoolOr,
	asJidString,
	asNumber,
	asString,
	bridgeJidToString,
	isBridgeJid,
	isObject,
	normalizeDiscriminator,
	toUnixSeconds
} from "./primitives.ts";

export type BridgeEventType = WhatsAppEvent["type"];
type BridgeData<T extends BridgeEventType> = Extract<
	WhatsAppEvent,
	{ type: T }
>["data"];

type AdapterFn<T extends BridgeEventType> = (
	data: BridgeData<T>,
	logger?: ILogger
) => CanonicalEvent | null;

type AdapterMap = { [K in BridgeEventType]: AdapterFn<K> };

const extractAction = (data: {
	action?: unknown;
}): Record<string, unknown> | undefined =>
	isObject(data.action) ? data.action : undefined;

const ADAPTERS = {
	connected: () => ({ type: "connected" }),
	disconnected: () => ({ type: "disconnected" }),
	stream_replaced: () => ({ type: "streamReplaced" }),
	client_outdated: () => ({ type: "clientOutdated" }),
	temporary_ban: data => ({
		type: "temporaryBan",
		code: asNumber(data?.code),
		expire: asNumber(data?.expire)
	}),
	qr_scanned_without_multidevice: () => ({
		type: "qrScannedWithoutMultidevice"
	}),
	logged_out: data => ({
		type: "loggedOut",
		onConnect: asBoolOr(data?.on_connect, false),
		reason: asString(data?.reason)
	}),

	qr: data => (data.code ? { type: "qr", code: data.code } : null),
	pairing_code: data => (data.code ? { type: "qr", code: data.code } : null),

	pair_success: data => {
		const id = typeof data.id === "string" ? data.id : asJidString(data.id);
		if (!id) return null;
		return {
			type: "pairSuccess",
			id,
			lid: typeof data.lid === "string" ? data.lid : asJidString(data.lid),
			platform: asString(data.platform),
			businessName: asString(data.business_name)
		};
	},

	pair_error: data => ({
		type: "pairError",
		error: asString(data.error) ?? "Unknown pairing error",
		id: typeof data.id === "string" ? data.id : asJidString(data.id),
		lid: typeof data.lid === "string" ? data.lid : asJidString(data.lid),
		businessName: asString(data.business_name),
		platform: asString(data.platform)
	}),

	connect_failure: data =>
		isObject(data)
			? {
					type: "connectFailure",
					message: asString(data.message),
					reason: asNumber(data.reason)
				}
			: { type: "connectFailure" },

	stream_error: data => ({
		type: "streamError",
		code: asString(data.code) ?? "unknown"
	}),

	message: (data, logger) => adaptMessage(data, logger),
	receipt: (data, logger) => adaptReceipt(data, logger),
	undecryptable_message: data => {
		if (!isObject(data)) return null;
		const info = isObject(data.info) ? data.info : undefined;
		if (!info) return null;
		const src = isObject(info.source) ? info.source : undefined;
		const chat = src && asJidString(src.chat);
		const id = asString(info.id);
		if (!src || !chat || !id) return null;
		const isGroup = asBoolOr(src.is_group, false);
		const participantAlt =
			isGroup && isBridgeJid(src.sender_alt)
				? bridgeJidToString(src.sender_alt)
				: undefined;
		const remoteJidAlt =
			!isGroup && isBridgeJid(src.recipient_alt)
				? bridgeJidToString(src.recipient_alt)
				: undefined;
		return {
			type: "undecryptableMessage",
			chatJid: chat,
			senderJid: isGroup ? asJidString(src.sender) : undefined,
			id,
			timestamp: toUnixSeconds(info.timestamp),
			isFromMe: asBoolOr(src.is_from_me, false),
			isGroup,
			pushName: asString(info.push_name),
			participantAlt,
			remoteJidAlt,
			isUnavailable: asBoolOr(data.is_unavailable, false),
			unavailableType: asString(data.unavailable_type),
			decryptFailMode: asString(data.decrypt_fail_mode),
			raw: data
		};
	},

	push_name_update: data => {
		const jid = asJidString(data.jid);
		return jid
			? {
					type: "pushNameUpdate",
					jid,
					newPushName: asString(data.new_push_name)
				}
			: null;
	},
	contact_update: data => adaptContactUpdate(data),
	contact_updated: data => adaptContactUpdate(data),

	picture_update: data => {
		const jid = asJidString(data.jid);
		if (!jid) return null;
		return {
			type: "pictureUpdate",
			jid,
			removed: asBoolOr(data.removed, false),
			author: asJidString(data.author),
			pictureId: asString(data.picture_id)
		};
	},

	presence: data => {
		const from = asJidString(data.from);
		if (!from) return null;
		return {
			type: "presence",
			from,
			unavailable: asBoolOr(data.unavailable, false),
			lastSeen: asNumber(data.last_seen)
		};
	},

	chat_presence: data => {
		const src = isObject(data.source) ? data.source : undefined;
		if (!src) return null;
		const chat = asJidString(src.chat);
		const sender = asJidString(src.sender);
		if (!chat || !sender) return null;

		const media = asString(data.media);

		const rawState = asString(data.state);
		if (rawState !== "composing" && rawState !== "paused") return null;
		return {
			type: "chatPresence",
			chatJid: chat,
			senderJid: sender,
			state: rawState,
			media: media === "audio" ? "audio" : undefined
		};
	},

	group_update: (data, logger) => adaptGroupUpdate(data, logger),

	archive_update: data => {
		const jid = asJidString(data.jid);
		if (!jid) return null;
		return {
			type: "archiveUpdate",
			jid,
			archived: asBoolOr(extractAction(data)?.archived, true)
		};
	},
	pin_update: data => {
		const jid = asJidString(data.jid);
		if (!jid) return null;
		return {
			type: "pinUpdate",
			jid,
			timestamp: asNumber(data.timestamp),
			pinned: asBoolOr(extractAction(data)?.pinned, true)
		};
	},
	mute_update: data => {
		const jid = asJidString(data.jid);
		if (!jid) return null;
		const action = extractAction(data);
		return {
			type: "muteUpdate",
			jid,
			timestamp: asNumber(data.timestamp),
			muted: asBoolOr(action?.muted, true),
			muteEndTimestamp:
				asNumber(action?.muteEndTimestamp) ??
				asNumber(action?.mute_end_timestamp)
		};
	},
	star_update: data => adaptStarUpdate(data),
	mark_chat_as_read_update: data => {
		const jid = asJidString(data.jid);
		if (!jid) return null;
		return {
			type: "markChatAsReadUpdate",
			jid,
			read: asBoolOr(extractAction(data)?.read, true)
		};
	},

	incoming_call: (data, logger) => adaptIncomingCall(data, logger),

	history_sync: data => {
		if (!isObject(data)) return null;
		const processed = processHistoryMessage(data as proto.IHistorySync);

		const overlay = data as Record<string, unknown>;
		const metaSyncType = asNumber(overlay.syncType);
		const metaChunkOrder = asNumber(overlay.chunkOrder);
		const metaProgress = asNumber(overlay.progress);
		const peerDataRequestSessionId = asString(overlay.peerDataRequestSessionId);
		return {
			type: "historySync",
			chats: processed.chats,
			contacts: processed.contacts,
			messages: processed.messages,
			lidPnMappings: processed.lidPnMappings,
			syncType: metaSyncType ?? processed.syncType,
			progress: metaProgress ?? processed.progress,
			chunkOrder: metaChunkOrder,
			peerDataRequestSessionId
		};
	},

	self_push_name_updated: () => ({
		type: "noop",
		bridgeType: "self_push_name_updated"
	}),
	offline_sync_completed: () => ({
		type: "noop",
		bridgeType: "offline_sync_completed"
	}),
	offline_sync_preview: () => ({
		type: "noop",
		bridgeType: "offline_sync_preview"
	}),
	device_list_update: () => ({
		type: "noop",
		bridgeType: "device_list_update"
	}),
	disappearing_mode_changed: data => {
		const jid = asJidString(data.from);
		const duration = asNumber(data.duration);
		if (!jid || duration == null)
			return { type: "noop", bridgeType: "disappearing_mode_changed" };
		return {
			type: "disappearingModeChanged",
			jid,
			duration,
			settingTimestamp: asNumber(data.setting_timestamp)
		};
	},
	business_status_update: () => ({
		type: "noop",
		bridgeType: "business_status_update"
	}),
	newsletter_live_update: data => {
		const newsletterJid = asJidString(data.newsletter_jid);
		if (!newsletterJid)
			return { type: "noop", bridgeType: "newsletter_live_update" };
		const rawMessages = Array.isArray(data.messages) ? data.messages : [];
		const messages = rawMessages
			.map(m => {
				if (!isObject(m)) return null;

				const serverId =
					asString(m.server_id) ?? asNumber(m.server_id)?.toString();
				if (!serverId) return null;
				const rawReactions = Array.isArray(m.reactions) ? m.reactions : [];
				const reactions = rawReactions
					.map(r => {
						if (!isObject(r)) return null;
						const code = asString(r.code);
						const count = asNumber(r.count);
						if (!code || count == null) return null;
						return { code, count };
					})
					.filter((r): r is { code: string; count: number } => r !== null);
				return { serverId, reactions };
			})
			.filter(
				(
					m
				): m is {
					serverId: string;
					reactions: { code: string; count: number }[];
				} => m !== null
			);
		return { type: "newsletterLiveUpdate", newsletterJid, messages };
	},
	contact_number_changed: data => {
		const oldJid = asJidString(data.old_jid);
		const newJid = asJidString(data.new_jid);
		const oldLid = asJidString(data.old_lid);
		const newLid = asJidString(data.new_lid);
		const mappings: { lid: string; pn: string }[] = [];
		if (oldLid && oldJid) mappings.push({ lid: oldLid, pn: oldJid });
		if (newLid && newJid) mappings.push({ lid: newLid, pn: newJid });
		if (mappings.length === 0)
			return { type: "noop", bridgeType: "contact_number_changed" };
		return { type: "lidMappingUpdate", mappings };
	},
	contact_sync_requested: () => ({
		type: "noop",
		bridgeType: "contact_sync_requested"
	}),
	user_about_update: () => ({ type: "noop", bridgeType: "user_about_update" }),
	delete_chat_update: data => {
		const jid = asJidString(data.jid);
		return jid
			? { type: "chatDelete", jid }
			: { type: "noop", bridgeType: "delete_chat_update" };
	},
	delete_message_for_me_update: data => {
		const chatJid = asJidString(data.chat_jid);
		const messageId = asString(data.message_id);
		if (!chatJid || !messageId)
			return { type: "noop", bridgeType: "delete_message_for_me_update" };
		return {
			type: "messageDelete",
			chatJid,
			messageId,
			fromMe: asBoolOr(data.from_me, false),
			participantJid: asJidString(data.participant_jid)
		};
	},

	notification: data => {
		if (!isObject(data)) return { type: "noop", bridgeType: "notification" };

		const attrs: Record<string, string> = {};
		if (isObject(data.attrs)) {
			for (const [k, v] of Object.entries(data.attrs)) {
				if (typeof v === "string") attrs[k] = v;
				else if (typeof v === "number" || typeof v === "boolean")
					attrs[k] = String(v);
			}
		}
		return {
			type: "notification",
			tag: asString(data.tag) ?? "notification",
			attrs
		};
	},

	raw_node: data => {
		if (!isObject(data) || typeof data.tag !== "string") return null;
		return { type: "rawNode", node: data as never };
	},

	mex_notification: data => {
		const opName = asString(data.op_name);
		if (!opName) return null;
		const payload = isObject(data.payload)
			? (data.payload as Record<string, unknown>)
			: {};
		return {
			type: "mexNotification",
			opName,
			from: asJidString(data.from),
			stanzaId: asString(data.stanza_id),
			offline: asBoolOr(data.offline, false),
			payload
		};
	}
} satisfies AdapterMap;

export const KNOWN_BRIDGE_EVENT_TYPES: ReadonlySet<string> = new Set(
	Object.keys(ADAPTERS)
);

export const adaptBridgeEventViaSchema = (
	event: WhatsAppEvent,
	logger?: ILogger
): CanonicalEvent | null => {
	const typed = event as { type: BridgeEventType; data?: unknown };
	const adapter = (ADAPTERS as Record<string, AdapterFn<BridgeEventType>>)[
		typed.type
	];
	if (!adapter) {
		logger?.debug(
			{ eventType: typed.type },
			"unknown bridge event (no canonical mapping)"
		);
		return null;
	}

	return adapter(typed.data as never, logger);
};

const adaptMessage = (
	data: BridgeData<"message">,
	logger?: ILogger
): CanonicalEvent | null => {
	if (!isObject(data)) return null;
	const info = isObject(data.info) ? data.info : undefined;
	const messageProto = isObject(data.message) ? data.message : undefined;
	if (!info || !messageProto) return null;

	const src = isObject(info.source) ? info.source : undefined;
	const chat = src && asJidString(src.chat);
	const id = asString(info.id);
	if (!src || !chat || !id) {
		logger?.debug({ info }, "message adapter: missing chat/id");
		return null;
	}

	const isGroup = asBoolOr(src.is_group, false);
	const senderRaw = src.sender;
	const senderJid = isGroup ? asJidString(senderRaw) : undefined;
	const participantAlt =
		isGroup && isBridgeJid(src.sender_alt)
			? bridgeJidToString(src.sender_alt)
			: undefined;
	const remoteJidAlt =
		!isGroup && isBridgeJid(src.recipient_alt)
			? bridgeJidToString(src.recipient_alt)
			: undefined;

	const editAttributeRaw = asString(info.edit);
	const editAttribute: "1" | "2" | "3" | "7" | "8" | undefined =
		editAttributeRaw === "1" ||
		editAttributeRaw === "2" ||
		editAttributeRaw === "3" ||
		editAttributeRaw === "7" ||
		editAttributeRaw === "8"
			? editAttributeRaw
			: undefined;
	return {
		type: "message",
		chatJid: chat,
		senderJid,
		isGroup,
		isFromMe: asBoolOr(src.is_from_me, false),
		id,
		timestamp: toUnixSeconds(info.timestamp),
		pushName: asString(info.push_name),
		participantAlt,
		remoteJidAlt,
		isViewOnce: info.is_view_once === true ? true : undefined,
		isOffline: asBoolOr(info.is_offline, false) ? true : undefined,
		unavailableRequestId: asString(info.unavailable_request_id),
		editAttribute,
		messageProto: messageProto as never
	};
};

const adaptReceipt = (
	data: BridgeData<"receipt">,
	logger?: ILogger
): CanonicalEvent | null => {
	if (!isObject(data)) return null;
	const src = isObject(data.source) ? data.source : undefined;
	if (!src) return null;
	const chat = asJidString(src.chat);
	const ids = Array.isArray(data.message_ids)
		? data.message_ids.filter((x): x is string => typeof x === "string")
		: [];
	if (!chat || ids.length === 0) {
		logger?.debug({ data }, "receipt adapter: missing chat or message_ids");
		return null;
	}
	const isGroup = asBoolOr(src.is_group, false);
	return {
		type: "receipt",
		chatJid: chat,
		senderJid: isGroup ? asJidString(src.sender) : undefined,
		isGroup,
		isFromMe: asBoolOr(src.is_from_me, false),
		messageIds: ids,
		timestamp: toUnixSeconds(data.timestamp),
		receiptType: parseReceiptType(data.type)
	};
};

const adaptContactUpdate = (data: unknown): CanonicalEvent | null => {
	if (!isObject(data)) return null;
	const jid = asJidString(data.jid);
	if (!jid) return null;
	const action = isObject(data.action) ? data.action : undefined;
	return {
		type: "contactUpdate",
		jid,
		fullName: asString(action?.fullName) ?? asString(action?.full_name),
		firstName: asString(action?.firstName) ?? asString(action?.first_name),
		lidJid: asString(action?.lidJid) ?? asString(action?.lid_jid),
		pnJid: asString(action?.pnJid) ?? asString(action?.pn_jid),
		username: asString(action?.username)
	};
};

const RECEIPT_TYPE_MAP: Record<
	string,
	NonNullable<import("./types.ts").CanonicalReceipt["receiptType"]>
> = {
	Delivered: "delivered",
	Sender: "sender",
	Retry: "retry",
	EncRekeyRetry: "enc-rekey-retry",
	Read: "read",
	ReadSelf: "read-self",
	Played: "played",
	PlayedSelf: "played-self",
	Inactive: "inactive",
	PeerMsg: "peer-msg",
	HistorySync: "history-sync",
	ServerError: "server-error",
	delivered: "delivered",
	sender: "sender",
	retry: "retry",
	enc_rekey_retry: "enc-rekey-retry",
	read: "read",
	read_self: "read-self",
	played: "played",
	played_self: "played-self",
	inactive: "inactive",
	peer_msg: "peer-msg",
	history_sync: "history-sync",
	server_error: "server-error"
};

const parseReceiptType = (
	raw: unknown
): import("./types.ts").CanonicalReceipt["receiptType"] => {
	const norm =
		typeof raw === "string"
			? raw
			: isObject(raw) && typeof raw.type === "string"
				? raw.type
				: undefined;
	if (norm == null) return undefined;
	return RECEIPT_TYPE_MAP[norm] ?? "other";
};

const adaptStarUpdate = (
	data: BridgeData<"star_update">
): CanonicalEvent | null => {
	if (!isObject(data)) return null;
	const chatJid = asJidString(data.chat_jid);
	const messageId = asString(data.message_id);
	if (!chatJid || !messageId) return null;
	const action = isObject(data.action)
		? (data.action as Record<string, unknown>)
		: undefined;
	return {
		type: "starUpdate",
		chatJid,
		messageId,
		fromMe: asBoolOr(data.from_me, false),
		participantJid: asJidString(data.participant_jid),
		starred: asBoolOr(action?.starred, false)
	};
};

const adaptIncomingCall = (
	data: BridgeData<"incoming_call">,
	logger?: ILogger
): CanonicalEvent | null => {
	if (!isObject(data)) return null;
	const from = asJidString(data.from);
	if (!from) return null;

	const action = isObject(data.action) ? data.action : undefined;
	if (!action) return null;

	const actionType = parseCallActionType(action.type);
	const callId = asString(action.call_id);
	if (!actionType || !callId) {
		logger?.debug(
			{ data },
			"incoming_call adapter: missing action.type/call_id"
		);
		return null;
	}

	const canonicalAction: CanonicalCallAction = { type: actionType, callId };

	const fields = action as Record<string, unknown>;
	if (actionType === "offer") {
		canonicalAction.callerPn = asJidString(fields.caller_pn);
		canonicalAction.callerCountryCode = asString(fields.caller_country_code);
		canonicalAction.deviceClass = asString(fields.device_class);
		canonicalAction.joinable = asBoolOr(fields.joinable, false);
		canonicalAction.isVideo = asBoolOr(fields.is_video, false);
		if (Array.isArray(fields.audio)) {
			canonicalAction.audio = fields.audio.filter(
				(x): x is string => typeof x === "string"
			);
		}
	} else if (actionType === "terminate") {
		canonicalAction.duration = asNumber(fields.duration);
		canonicalAction.audioDuration = asNumber(fields.audio_duration);
	}

	return {
		type: "incomingCall",
		from,
		timestamp: toUnixSeconds(data.timestamp),
		offline: asBoolOr(data.offline, false),
		stanzaId: asString(data.stanza_id),
		notify: asString(data.notify),
		platform: asString(data.platform),
		version: asString(data.version),
		action: canonicalAction
	};
};

const parseCallActionType = (
	raw: unknown
): CanonicalCallActionType | undefined => {
	const norm = normalizeDiscriminator(raw);
	switch (norm) {
		case "offer":
			return "offer";
		case "pre_accept":
		case "preaccept":
			return "preAccept";
		case "accept":
			return "accept";
		case "reject":
			return "reject";
		case "terminate":
			return "terminate";
		default:
			return undefined;
	}
};

const adaptGroupParticipant = (
	raw: unknown
): CanonicalGroupParticipant | null => {
	if (!isObject(raw)) return null;
	const jid = asJidString(raw.jid);
	if (!jid) return null;
	return { jid, phoneNumber: asJidString(raw.phone_number) };
};

const adaptGroupParticipants = (raw: unknown): CanonicalGroupParticipant[] => {
	if (!Array.isArray(raw)) return [];
	return raw
		.map(adaptGroupParticipant)
		.filter((p): p is CanonicalGroupParticipant => p !== null);
};

const adaptGroupAction = (raw: unknown): CanonicalGroupAction | null => {
	if (!isObject(raw)) return null;
	const norm = normalizeDiscriminator(raw.type);
	const rawType = asString(raw.type) ?? "unknown";
	if (!norm) return { type: "unknown", rawType };

	switch (norm) {
		case "add":
			return {
				type: "add",
				participants: adaptGroupParticipants(raw.participants),
				reason: asString(raw.reason)
			};
		case "remove":
			return {
				type: "remove",
				participants: adaptGroupParticipants(raw.participants),
				reason: asString(raw.reason)
			};
		case "promote":
			return {
				type: "promote",
				participants: adaptGroupParticipants(raw.participants)
			};
		case "demote":
			return {
				type: "demote",
				participants: adaptGroupParticipants(raw.participants)
			};
		case "modify":
			return {
				type: "modify",
				participants: adaptGroupParticipants(raw.participants)
			};
		case "subject":
			return {
				type: "subject",
				subject: asString(raw.subject) ?? "",
				subjectOwner: asJidString(raw.subject_owner),
				subjectTime: asNumber(raw.subject_time)
			};
		case "description":
			return {
				type: "description",
				id: asString(raw.id) ?? "",
				description: asString(raw.description)
			};
		case "locked":
			return { type: "locked" };
		case "unlocked":
			return { type: "unlocked" };
		case "announce":
		case "announcement":
			return { type: "announce" };
		case "not_announce":
		case "not_announcement":
		case "notannounce":
			return { type: "notAnnounce" };
		case "ephemeral": {
			const expiration = asNumber(raw.expiration);
			if (expiration == null) return null;
			return { type: "ephemeral", expiration, trigger: asNumber(raw.trigger) };
		}
		case "membership_approval_mode":
		case "membershipapprovalmode":
			return {
				type: "membershipApprovalMode",
				enabled: asBoolOr(raw.enabled, false)
			};
		case "member_add_mode":
		case "memberaddmode":
			return { type: "memberAddMode", mode: asString(raw.mode) ?? "" };
		case "no_frequently_forwarded":
		case "nofrequentlyforwarded":
			return { type: "noFrequentlyForwarded" };
		case "frequently_forwarded_ok":
		case "frequentlyforwardedok":
			return { type: "frequentlyForwardedOk" };
		case "invite":
			return { type: "invite", code: asString(raw.code) ?? "" };
		case "revoke":
		case "revoke_invite":
		case "revokeinvite":
			return { type: "revokeInvite" };
		case "create":
			return { type: "create" };
		case "delete":
			return { type: "delete", reason: asString(raw.reason) };
		case "link":
			return { type: "link", linkType: asString(raw.link_type) ?? "" };
		case "unlink":
			return {
				type: "unlink",
				unlinkType: asString(raw.unlink_type) ?? "",
				unlinkReason: asString(raw.unlink_reason)
			};
		case "growth_locked":
		case "growthlocked": {
			const expiration = asNumber(raw.expiration);
			if (expiration == null) return null;
			return {
				type: "growthLocked",
				expiration,
				lockType: asString(raw.lock_type) ?? ""
			};
		}
		case "growth_unlocked":
		case "growthunlocked":
			return { type: "growthUnlocked" };
		case "membership_approval_request":
			return {
				type: "membershipApprovalRequest",
				requestMethod: asString(raw.request_method),
				parentGroupJid: asJidString(raw.parent_group_jid)
			};
		case "created_membership_requests":
			return {
				type: "createdMembershipRequests",
				requestMethod: asString(raw.request_method),
				parentGroupJid: asJidString(raw.parent_group_jid),
				requests: adaptGroupParticipants(raw.requests)
			};
		case "revoked_membership_requests":
			return {
				type: "revokedMembershipRequests",

				participants: Array.isArray(raw.participants)
					? raw.participants
							.map(j => {
								const jid = asJidString(j);
								return jid ? { jid } : null;
							})
							.filter((p): p is CanonicalGroupParticipant => p !== null)
					: []
			};
		default:
			return { type: "unknown", rawType };
	}
};

const adaptGroupUpdate = (
	data: BridgeData<"group_update">,
	logger?: ILogger
): CanonicalEvent | null => {
	if (!isObject(data)) return null;
	const groupJid = asJidString(data.group_jid);
	if (!groupJid) return null;
	const action = adaptGroupAction(data.action);
	if (!action) {
		logger?.warn({ data }, "group_update adapter: action shape rejected");
		return null;
	}
	return {
		type: "groupUpdate",
		groupJid,
		author: asJidString(data.participant),
		authorPn: asJidString(data.participant_pn),
		timestamp: toUnixSeconds(data.timestamp),
		isLidAddressingMode: asBoolOr(data.is_lid_addressing_mode, false),
		action
	};
};
