const https = require("https");
const fs = require("fs");
const path = require("path");
const { execFile, execFileSync } = require("child_process");
const { pipeline } = require("stream/promises");

const RELEASE_URL =
	"https://github.com/Thruqe/whatsrook/releases/download/alpha/whatsrook-linux-amd64.tar.gz";
const DEST_DIR = path.join(__dirname, "whatsrook-bin");
const TAR_PATH = path.join(__dirname, "whatsrook.tar.gz");

const SESSION = process.env.SESSION || ""; // phone number for the session, e.g. "2348012345678" (digits only, country code included, required unless updating)
const CLIENT = process.env.CLIENT || "chrome"; // client type: chrome (default), android, or ios if you want to be able to read viewonce

function ensureTarDependency() {
	try {
		require.resolve("tar");
	} catch {
		console.log('Installing "tar" package...');
		execFileSync("npm", ["install", "tar"], {
			cwd: __dirname,
			stdio: "inherit",
		});
	}
	return require("tar");
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

function run(binaryPath, args = []) {
	return new Promise((resolve, reject) => {
		fs.chmodSync(binaryPath, 0o755);
		const child = execFile(binaryPath, args, { cwd: path.dirname(binaryPath) });
		child.stdout.on("data", (d) => process.stdout.write(d));
		child.stderr.on("data", (d) => process.stderr.write(d));
		child.on("exit", (code) =>
			code === 0 ? resolve() : reject(new Error(`Exited with code ${code}`)),
		);
	});
}

async function main() {
	if (!SESSION) {
		throw new Error(
			'SESSION environment variable is required (phone number, e.g. "2348012345678")',
		);
	}

	const tar = ensureTarDependency();

	console.log("Downloading release...");
	await download(RELEASE_URL, TAR_PATH);

	console.log("Extracting...");
	await extract(tar, TAR_PATH, DEST_DIR);

	fs.unlinkSync(TAR_PATH); // free up space once extracted

	const binaryPath = path.join(DEST_DIR, "whatsrook");

	const args = [
		"--session",
		SESSION,
		"--port",
		PORT,
		"--auth-dir",
		AUTH_DIR,
		"--client",
		CLIENT,
	];

	console.log("Running...");
	await run(binaryPath, args);
}

main().catch((err) => {
	console.error("Failed:", err);
	process.exit(1);
});
