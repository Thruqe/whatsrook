/**
 * Anti-corruption layer between bridge runtime and baileyrs domain code.
 *
 * Adapters here normalize raw bridge payloads into the canonical event union
 * declared in `types.ts`. Downstream code consumes the canonical types
 * exclusively — see `Socket/events.ts` for the bridge-canonical-Baileys
 * three-stage pipeline.
 */
export { adaptBridgeEvent } from "./adapt.ts";
export { KNOWN_BRIDGE_EVENT_TYPES } from "./constants.ts";
export * from "./types.ts";
export {
	asBool,
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
