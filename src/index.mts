import makeWASocket from "./Socket/index.mts";

export { proto, proto as WAProto } from "whatsapp-rust-bridge/proto-types";
export * from "./Utils/index.mts";
export * from "./Types/index.mts";
export * from "./Defaults/index.mts";

export type WASocket = ReturnType<typeof makeWASocket>;
export { makeWASocket };
export default makeWASocket;
