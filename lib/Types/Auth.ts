import { Buffer } from "node:buffer";
import type { JsStoreCallbacks } from "whatsapp-rust-bridge";
import type { proto } from "whatsapp-rust-bridge/proto-types";
import type { Contact } from "./Contact.ts";

export type KeyPair = { public: Uint8Array; private: Uint8Array };
export type SignedKeyPair = {
  keyPair: KeyPair;
  signature: Uint8Array;
  keyId: number;
  timestampS?: number;
};

export type ProtocolAddress = {
  name: string;
  deviceId: number;
};
export type SignalIdentity = {
  identifier: ProtocolAddress;
  identifierKey: Uint8Array;
};

export type LIDMapping = {
  pn: string;
  lid: string;
};

export type LTHashState = {
  version: number;
  hash: Buffer;
  indexValueMap: {
    [indexMacBase64: string]: { valueMac: Uint8Array | Buffer };
  };
};

export type AccountSettings = {
  /** unarchive chats when a new message is received */
  unarchiveChats: boolean;
  /** the default mode to start new conversations with */
  defaultDisappearingMode?: Pick<
    proto.IConversation,
    "ephemeralExpiration" | "ephemeralSettingTimestamp"
  >;
};

/**
 * Authentication credentials.
 *
 * Only `me`, `registered`, and `platform` are actively used by the socket.
 * All crypto keys and Signal state are managed internally by the Rust bridge.
 * The remaining fields exist for backward compatibility with user code that
 * may reference them.
 */
export type AuthenticationCreds = {
  /** Paired user identity — set on pair_success, cleared on logout. */
  me?: Contact;
  /** Whether the client has completed pairing. */
  registered: boolean;
  /** Device platform (e.g. "smbi", "smba"). Set on pair_success. */
  platform?: string;
  noiseKey?: KeyPair;
  pairingEphemeralKeyPair?: KeyPair;
  signedIdentityKey?: KeyPair;
  signedPreKey?: SignedKeyPair;
  registrationId?: number;
  advSecretKey?: string;
  account?: proto.IAdvSignedDeviceIdentity;
  signalIdentities?: SignalIdentity[];
  myAppStateKeyId?: string;
  firstUnuploadedPreKeyId?: number;
  nextPreKeyId?: number;
  lastAccountSyncTimestamp?: number;
  processedHistoryMessages?: Array<{
    key: { remoteJid?: string; id?: string };
    messageTimestamp?: number;
  }>;
  accountSyncCounter?: number;
  accountSettings?: AccountSettings;
  pairingCode?: string;
  lastPropHash?: string;
  routingInfo?: Buffer;
  additionalData?: Record<string, unknown>;
};

export type AuthenticationState = {
  /** Bridge storage for persistent WASM state. Auto-created by useMultiFileAuthState. */
  store?: JsStoreCallbacks & {
    /** Flush all pending debounced writes to disk. Called automatically on disconnect. */
    flush?(): Promise<void>;
  };
};
