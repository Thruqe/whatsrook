const ANSI = {
	reset: "\x1b[0m",
	bold: "\x1b[1m",
	cyan: "\x1b[36m",
	green: "\x1b[32m",
	yellow: "\x1b[33m",
	red: "\x1b[31m",
	magenta: "\x1b[35m",
	blue: "\x1b[34m",
	white: "\x1b[37m",
	gray: "\x1b[90m"
};

const LEVEL_VALUES: Record<string, number> = {
	trace: 10,
	debug: 20,
	info: 30,
	warn: 40,
	error: 50,
	fatal: 60,
	silent: Infinity
};

const LEVEL_STYLE: Record<string, { label: string; color: string }> = {
	trace: { label: "TRACE", color: ANSI.gray },
	debug: { label: "DEBUG", color: ANSI.blue },
	info: { label: "INFO ", color: ANSI.green },
	warn: { label: "WARN ", color: ANSI.yellow },
	error: { label: "ERROR", color: ANSI.red },
	fatal: { label: "FATAL", color: ANSI.magenta }
};

const REDACTED_KEYS = new Set([
	"creds",
	"authState",
	"noiseKey",
	"signedIdentityKey",
	"advSecretKey",
	"privateKey",
	"secretKey",
	"preKey"
]);

const WILDCARD_REDACTED_KEYS = new Set([
	"creds",
	"authState",
	"noiseKey",
	"signedIdentityKey",
	"advSecretKey",
	"privateKey",
	"secretKey"
]);

export interface ILogger {
	level: string;
	child(obj: Record<string, unknown>): ILogger;
	trace(obj: unknown, msg?: string): void;
	debug(obj: unknown, msg?: string): void;
	info(obj: unknown, msg?: string): void;
	warn(obj: unknown, msg?: string): void;
	error(obj: unknown, msg?: string): void;
}

function redactObject(obj: Record<string, unknown>): Record<string, unknown> {
	const result: Record<string, unknown> = {};
	for (const [key, value] of Object.entries(obj)) {
		if (REDACTED_KEYS.has(key)) {
			result[key] = "[REDACTED]";
		} else if (value && typeof value === "object" && !Array.isArray(value)) {
			const nested = value as Record<string, unknown>;
			const redactedNested: Record<string, unknown> = {};
			for (const [nk, nv] of Object.entries(nested)) {
				redactedNested[nk] = WILDCARD_REDACTED_KEYS.has(nk) ? "[REDACTED]" : nv;
			}
			result[key] = redactedNested;
		} else {
			result[key] = value;
		}
	}
	return result;
}

function formatExtra(obj: Record<string, unknown>): string {
	const skip = new Set(["msg", "level", "time", "name", "pid", "hostname"]);
	const parts: string[] = [];
	for (const [key, value] of Object.entries(obj)) {
		if (skip.has(key)) continue;
		const val =
			typeof value === "object" ? JSON.stringify(value) : String(value);
		parts.push(
			`${ANSI.gray}${key}${ANSI.reset}=${ANSI.cyan}${val}${ANSI.reset}`
		);
	}
	return parts.length ? " " + parts.join(" ") : "";
}

function write(
	name: string,
	bindings: Record<string, unknown>,
	msgLevel: string,
	currentLevel: string,
	obj: unknown,
	msg?: string
): void {
	if ((LEVEL_VALUES[msgLevel] ?? 0) < (LEVEL_VALUES[currentLevel] ?? 0)) return;

	const style = LEVEL_STYLE[msgLevel] ?? {
		label: msgLevel.toUpperCase(),
		color: ANSI.white
	};
	const time = new Date().toLocaleTimeString("en-US", { hour12: false });
	const levelStr = `${style.color}${ANSI.bold}${style.label}${ANSI.reset}`;
	const timeStr = `${ANSI.gray}${time}${ANSI.reset}`;
	const nameStr = `${ANSI.cyan}(${name})${ANSI.reset}`;

	let message = "";
	const extra = { ...bindings };

	if (obj && typeof obj === "object") {
		const redacted = redactObject(obj as Record<string, unknown>);
		Object.assign(extra, redacted);
		message = msg ?? (extra.msg as string) ?? "";
		delete extra.msg;
	} else if (typeof obj === "string") {
		message = msg ? `${obj} ${msg}` : obj;
	} else {
		message = msg ?? "";
	}

	const line = `${timeStr} ${levelStr} ${nameStr}: ${ANSI.white}${message}${ANSI.reset}${formatExtra(extra)}`;

	if (msgLevel === "error" || msgLevel === "fatal") {
		process.stderr.write(line + "\n");
	} else {
		process.stdout.write(line + "\n");
	}
}

function createLogger(
	level: string,
	bindings: Record<string, unknown> = {},
	name = "logger"
): ILogger {
	return {
		level,
		child(obj) {
			return createLogger(level, { ...bindings, ...obj }, name);
		},
		trace(obj, msg) {
			write(name, bindings, "trace", level, obj, msg);
		},
		debug(obj, msg) {
			write(name, bindings, "debug", level, obj, msg);
		},
		info(obj, msg) {
			write(name, bindings, "info", level, obj, msg);
		},
		warn(obj, msg) {
			write(name, bindings, "warn", level, obj, msg);
		},
		error(obj, msg) {
			write(name, bindings, "error", level, obj, msg);
		}
	};
}

export { createLogger };
