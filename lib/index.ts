import makeWASocket from "./Socket/index.ts";

export { proto, proto as WAProto } from "whatsapp-rust-bridge/proto-types";
export * from "./Utils/index.ts";
export * from "./Types/index.ts";
export * from "./Defaults/index.ts";

export type WASocket = ReturnType<typeof makeWASocket>;
export { makeWASocket };
export default makeWASocket;
