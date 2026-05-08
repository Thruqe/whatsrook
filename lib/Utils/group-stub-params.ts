declare const StubParticipantBrand: unique symbol;

export type StubParticipantParam = string & {
	readonly [StubParticipantBrand]: never;
};

export interface StubParticipantPayload {
	id: string;
	phoneNumber?: string;
}

const isNonEmptyString = (v: unknown): v is string =>
	typeof v === "string" && v !== "";

export const encodeStubParticipant = (
	p: StubParticipantPayload
): StubParticipantParam => {
	const out: StubParticipantPayload = { id: p.id };
	if (isNonEmptyString(p.phoneNumber)) out.phoneNumber = p.phoneNumber;
	return JSON.stringify(out) as StubParticipantParam;
};

/**
 * Parse a raw `messageStubParameters[i]` back into its payload. Validates
 * the shape so a malformed/legacy raw-JID string fails loudly instead of
 * propagating a wrong type. Returns `null` on any decode failure — callers
 * decide whether that is an assertion failure or a fall-through.
 */
export const decodeStubParticipant = (
	raw: string | null | undefined
): StubParticipantPayload | null => {
	if (!raw) return null;
	let parsed: unknown;
	try {
		parsed = JSON.parse(raw);
	} catch {
		return null;
	}
	if (parsed === null || typeof parsed !== "object") return null;
	const obj = parsed as Record<string, unknown>;
	if (typeof obj.id !== "string") return null;
	const out: StubParticipantPayload = { id: obj.id };
	if (isNonEmptyString(obj.phoneNumber)) out.phoneNumber = obj.phoneNumber;
	return out;
};
