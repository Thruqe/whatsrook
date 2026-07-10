import type { BinaryNode } from "../Types/index.ts";
import { Boom } from "../Utils/boom.ts";

export const getBinaryNodeChildren = (
	node: BinaryNode | undefined,
	childTag: string
): BinaryNode[] => {
	if (Array.isArray(node?.content)) {
		return node.content.filter(
			item => (item as BinaryNode).tag === childTag
		) as BinaryNode[];
	}
	return [];
};

export const getBinaryNodeChild = (
	node: BinaryNode | undefined,
	childTag: string
): BinaryNode | undefined => {
	if (Array.isArray(node?.content)) {
		return node.content.find(item => (item as BinaryNode).tag === childTag) as
			| BinaryNode
			| undefined;
	}
	return undefined;
};

/**
 * Throws a `Boom` when the response stanza carries an `<error>` child.
 * Mirrors upstream `assertNodeErrorFree` — used in IQ response paths
 * where the server signals failure via a sibling `<error>` instead of
 * a top-level rejection.
 */
export const assertNodeErrorFree = (node: BinaryNode): void => {
	const errNode = getBinaryNodeChild(node, "error");
	if (errNode) {
		const code = errNode.attrs?.code;
		// Single parse for both `statusCode` and `data` — the previous
		// implementation parsed twice (parseInt + Number) which could
		// disagree on edge cases (e.g. `Number('')` is 0 but
		// `parseInt('', 10)` is NaN). When the server sent a non-numeric
		// code, fall back to 500 for statusCode but preserve the original
		// string in `data` so callers can still inspect the wire value.
		const parsed =
			typeof code === "string" ? Number.parseInt(code, 10) : undefined;
		const statusCode = Number.isFinite(parsed) ? (parsed as number) : 500;
		throw new Boom(errNode.attrs?.text || "Unknown error", {
			statusCode,
			data: Number.isFinite(parsed) ? parsed : code
		});
	}
};
