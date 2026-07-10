import P from "pino";

export interface ILogger {
  level: string;
  child(obj: Record<string, unknown>): ILogger;
  trace(obj: unknown, msg?: string): void;
  debug(obj: unknown, msg?: string): void;
  info(obj: unknown, msg?: string): void;
  warn(obj: unknown, msg?: string): void;
  error(obj: unknown, msg?: string): void;
}

export default P({
  name: "logger",
  level: process.env.LOG_LEVEL || "info",
  timestamp: () => `,"time":"${new Date().toJSON()}"`,
  redact: {
    paths: [
      "creds",
      "authState",
      "noiseKey",
      "signedIdentityKey",
      "advSecretKey",
      "privateKey",
      "secretKey",
      "preKey.privateKey",
      "*.creds",
      "*.authState",
      "*.noiseKey",
      "*.signedIdentityKey",
      "*.advSecretKey",
      "*.privateKey",
      "*.secretKey",
    ],
    censor: "[REDACTED]",
  },
});
