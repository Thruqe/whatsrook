const https = require("https");
const fs = require("fs");
const path = require("path");
const { spawn, execFileSync } = require("child_process");
const { pipeline } = require("stream/promises");

const CLIENT = process.env.CLIENT || "ios"; // client type: chrome (default), android, or ios if you want to be able to read viewonce

const RELEASE_URL =
	"https://github.com/Thruqe/whatsrook/releases/download/alpha/whatsrook-linux-amd64.tar.gz";
const DEST_DIR = path.join(__dirname, "whatsrook-bin");
const TAR_PATH = path.join(__dirname, "whatsrook.tar.gz");
const BINARY_PATH = path.join(DEST_DIR, "whatsrook");

const YTDLP_URL =
	"https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp";
const YTDLP_DIR = path.join(__dirname, "bin");
const YTDLP_PATH = path.join(YTDLP_DIR, "yt-dlp");

const PHONE_FILE = path.join(__dirname, "phone.txt");
const LOG_DIR = path.join(__dirname, "logs");

const sessions = new Map(); // phone -> { proc, pairCode, appInfo, clientInfo }

function ensureTarDependency() {
	try {
		require.resolve("tar");
	} catch {
		execFileSync("npm", ["install", "tar"], {
			cwd: __dirname,
			stdio: "inherit",
		});
	}
	return require("tar");
}

function ensureYtDlp() {
	fs.mkdirSync(YTDLP_DIR, { recursive: true });
	if (fs.existsSync(YTDLP_PATH)) return;
	execFileSync("curl", ["-L", YTDLP_URL, "-o", YTDLP_PATH], {
		stdio: "inherit",
	});
	fs.chmodSync(YTDLP_PATH, 0o755);
}

function download(url, dest) {
	return new Promise((resolve, reject) => {
		https
			.get(url, (res) => {
				if (
					res.statusCode >= 300 &&
					res.statusCode < 400 &&
					res.headers.location
				) {
					return download(res.headers.location, dest).then(resolve, reject);
				}
				if (res.statusCode !== 200) {
					return reject(new Error(`Download failed: ${res.statusCode}`));
				}
				const fileStream = fs.createWriteStream(dest);
				pipeline(res, fileStream).then(resolve, reject);
			})
			.on("error", reject);
	});
}

async function extract(tar, tarPath, destDir) {
	fs.mkdirSync(destDir, { recursive: true });
	await tar.x({ file: tarPath, cwd: destDir });
}

function readPhoneList() {
	if (!fs.existsSync(PHONE_FILE)) return [];
	return fs
		.readFileSync(PHONE_FILE, "utf8")
		.split("\n")
		.map((line) => line.trim())
		.filter(Boolean);
}

function printRow(phone, label, value) {
	console.log(`SESSION ${phone.padEnd(15)} | ${label.padEnd(11)} | ${value}`);
}

function parseLine(phone, line) {
	const state = sessions.get(phone);
	if (!state) return;

	const appInfoMatch = line.match(/\[App INFO\]\s+(.+)/i);
	const clientInfoMatch = line.match(/\[Client INFO\]\s+(.+)/i);
	const pairCodeMatch = line.match(
		/Enter this code on your phone:\s*([A-Z0-9-]+)/i,
	);

	if (appInfoMatch) {
		state.appInfo = appInfoMatch[1].trim();
		printRow(phone, "APP INFO", state.appInfo);
	}
	if (clientInfoMatch) {
		state.clientInfo = clientInfoMatch[1].trim();
		printRow(phone, "CLIENT INFO", state.clientInfo);
	}
	if (pairCodeMatch) {
		state.pairCode = pairCodeMatch[1].trim();
		printRow(phone, "PAIR CODE", state.pairCode);
	}
}

function startSession(phone) {
	if (sessions.has(phone)) return;

	fs.mkdirSync(LOG_DIR, { recursive: true });
	const logPath = path.join(LOG_DIR, `${phone}.log`);
	const logStream = fs.createWriteStream(logPath, { flags: "a" });

	const args = ["--session", phone, "--client", CLIENT, "-p"];

	const proc = spawn(BINARY_PATH, args, {
		cwd: DEST_DIR,
		env: {
			...process.env,
			PATH: `${YTDLP_DIR}:${process.env.PATH}`,
		},
	});

	const state = { proc, pairCode: null, appInfo: null, clientInfo: null };
	sessions.set(phone, state);

	let stdoutBuf = "";
	proc.stdout.on("data", (chunk) => {
		logStream.write(chunk);
		stdoutBuf += chunk.toString();
		let idx;
		while ((idx = stdoutBuf.indexOf("\n")) >= 0) {
			const line = stdoutBuf.slice(0, idx);
			stdoutBuf = stdoutBuf.slice(idx + 1);
			parseLine(phone, line);
		}
	});

	let stderrBuf = "";
	proc.stderr.on("data", (chunk) => {
		logStream.write(chunk);
		stderrBuf += chunk.toString();
		let idx;
		while ((idx = stderrBuf.indexOf("\n")) >= 0) {
			const line = stderrBuf.slice(0, idx);
			stderrBuf = stderrBuf.slice(idx + 1);
			parseLine(phone, line);
		}
	});

	proc.on("exit", () => {
		logStream.end();
	});
}

function stopSession(phone) {
	const state = sessions.get(phone);
	if (!state) return;

	try {
		execFileSync(BINARY_PATH, ["--session", phone, "--client", CLIENT, "-l"], {
			cwd: DEST_DIR,
			stdio: "ignore",
		});
	} catch {}

	if (!state.proc.killed) {
		state.proc.kill();
	}
	sessions.delete(phone);
}

function reconcile() {
	const current = new Set(readPhoneList());

	for (const phone of current) {
		if (!sessions.has(phone)) {
			startSession(phone);
		}
	}

	for (const phone of sessions.keys()) {
		if (!current.has(phone)) {
			stopSession(phone);
		}
	}
}

async function main() {
	const tar = ensureTarDependency();
	ensureYtDlp();

	await download(RELEASE_URL, TAR_PATH);
	await extract(tar, TAR_PATH, DEST_DIR);
	fs.unlinkSync(TAR_PATH);

	reconcile();

	fs.watch(PHONE_FILE, { persistent: true }, () => {
		reconcile();
	});
}

main().catch((err) => {
	console.error("Failed:", err);
	process.exit(1);
});
