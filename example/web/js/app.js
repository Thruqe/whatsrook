// Vanilla JS UI logic: phone entry -> QR/pair choice -> pairing -> dashboard.

const screens = {
	phone: document.getElementById("screen-phone"),
	choice: document.getElementById("screen-choice"),
	qr: document.getElementById("screen-qr"),
	code: document.getElementById("screen-code"),
	dashboard: document.getElementById("screen-dashboard"),
};

function showScreen(name) {
	Object.values(screens).forEach((el) => el.classList.remove("active"));
	screens[name].classList.add("active");
}

let currentPhoneNumber = "";

// --- Screen 1: phone number ------------------------------------------------

const phoneInput = document.getElementById("phone-number");
const btnContinue = document.getElementById("btn-continue");
const btnContinueText = document.getElementById("btn-continue-text");
const phoneError = document.getElementById("phone-error");
const phoneErrorText = document.getElementById("phone-error-text");

function setPhoneError(msg) {
	phoneErrorText.textContent = msg;
	phoneError.classList.add("show");
}
function clearPhoneError() {
	phoneError.classList.remove("show");
}

btnContinue.addEventListener("click", () => {
	const value = phoneInput.value.replace(/\D/g, "");
	clearPhoneError();

	if (value.length < 8) {
		setPhoneError("Please enter a valid phone number.");
		return;
	}

	currentPhoneNumber = value;
	btnContinue.disabled = true;
	btnContinueText.innerHTML = `<span class="spinner"></span>`;

	// Ask current status; if already logged in for this session, skip to dashboard.
	WSBridge.send("get_status");

	// Give it a moment to respond, otherwise proceed to the choice screen.
	setTimeout(() => {
		btnContinue.disabled = false;
		btnContinueText.textContent = "Continue";
		if (!screens.dashboard.classList.contains("active")) {
			document.getElementById("dash-number").textContent =
				`+${currentPhoneNumber}`;
			showScreen("choice");
		}
	}, 700);
});

phoneInput.addEventListener("keydown", (e) => {
	if (e.key === "Enter") btnContinue.click();
});

// --- Screen 2: choice -------------------------------------------------------

document
	.getElementById("choice-back")
	.addEventListener("click", () => showScreen("phone"));

document.getElementById("choice-qr").addEventListener("click", () => {
	showScreen("qr");
	document.getElementById("qr-status-text").textContent = "Requesting code…";
	WSBridge.send("request_pair_qr");
});

document.getElementById("choice-pair").addEventListener("click", () => {
	showScreen("code");
	document.getElementById("code-status-text").textContent = "Requesting code…";
	WSBridge.send("request_pair_code", { phoneNumber: currentPhoneNumber });
});

document
	.getElementById("qr-back")
	.addEventListener("click", () => showScreen("choice"));
document
	.getElementById("code-back")
	.addEventListener("click", () => showScreen("choice"));

// --- Bridge events -----------------------------------------------------------

WSBridge.on("pair_qr", ({ code }) => {
	const wrap = document.getElementById("qr-canvas-wrap");
	// Render the QR using a lightweight public QR image service (no extra deps).
	wrap.innerHTML = `<img src="https://api.qrserver.com/v1/create-qr-code/?size=220x220&data=${encodeURIComponent(code)}" alt="QR code" />`;
	document.getElementById("qr-status-text").textContent = "Waiting for scan…";
});

WSBridge.on("pair_code", ({ code }) => {
	const chunks = code.match(/.{1,4}/g) ?? [code];
	document.getElementById("pair-code-display").innerHTML = chunks
		.map((c) => `<span>${c}</span>`)
		.join("");
	document.getElementById("code-status-text").textContent =
		"Waiting for confirmation…";
});

WSBridge.on("pair_error", ({ reason }) => {
	const activeId = screens.qr.classList.contains("active")
		? "qr-status-text"
		: "code-status-text";
	document.getElementById(activeId).textContent =
		"Something went wrong. Please try again.";
});

WSBridge.on("status", (data) => {
	if (data.connected && data.loggedIn) {
		enterDashboard(data);
	}
});

function enterDashboard(data) {
	document.getElementById("dash-name").textContent =
		data.pushName || "Connected";
	document.getElementById("dash-number").textContent = data.jid
		? `+${data.jid.split("@")[0]}`
		: `+${currentPhoneNumber}`;
	document.getElementById("dash-conn-status").textContent = "Connected";
	showScreen("dashboard");
}

document.getElementById("btn-logout").addEventListener("click", () => {
	WSBridge.send("logout");
	showScreen("phone");
	phoneInput.value = "";
});

// --- Dashboard activity feed -------------------------------------------------

const feed = document.getElementById("dash-feed");
let feedHasItems = false;

function addFeedItem(title, body) {
	if (!feedHasItems) {
		feed.innerHTML = "";
		feedHasItems = true;
	}
	const item = document.createElement("div");
	item.className = "feed-item";
	item.innerHTML = `
    <div class="top"><span>${title}</span><span>${new Date().toLocaleTimeString()}</span></div>
    <div class="body">${body}</div>
  `;
	feed.prepend(item);
}

WSBridge.on("message", (m) => {
	addFeedItem(m.pushName || m.sender, m.text || "(media)");
});

WSBridge.on("incoming_call", (c) => {
	addFeedItem("Incoming call", c.from);
});
