import type { WhatsAppEvent } from "whatsapp-rust-bridge";
import type { ILogger } from "../Utils/logger.ts";
import type { CanonicalEvent } from "./types.ts";
import {
	adaptBridgeEventViaSchema,
	KNOWN_BRIDGE_EVENT_TYPES as KNOWN_FROM_SCHEMA
} from "./schema.ts";

/**
 * Set of bridge event types the adapter explicitly handles.
 */
export const KNOWN_BRIDGE_EVENT_TYPES: ReadonlySet<string> = KNOWN_FROM_SCHEMA;

export const adaptBridgeEvent = (
	event: WhatsAppEvent,
	logger?: ILogger
): CanonicalEvent | null => adaptBridgeEventViaSchema(event, logger);
