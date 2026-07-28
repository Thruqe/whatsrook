// Thin wrapper around the browser-facing WebSocket bridge served by client.ts.
// Emits friendly events; no protobuf knowledge lives here.

const WSBridge = (() => {
	let socket = null;
	const listeners = {};

	function connect() {
		const proto = location.protocol === "https:" ? "wss" : "ws";
		socket = new WebSocket(`${proto}://${location.host}/ws`);

		socket.addEventListener("open", () => emit("bridge_open", {}));
		socket.addEventListener("close", () => {
			emit("bridge_close", {});
			setTimeout(connect, 2000);
		});
		socket.addEventListener("message", (event) => {
			let msg;
			try {
				msg = JSON.parse(event.data);
			} catch {
				return;
			}
			emit(msg.type, msg.data);
		});
	}

	function on(type, cb) {
		(listeners[type] ??= []).push(cb);
	}

	function emit(type, data) {
		(listeners[type] ?? []).forEach((cb) => cb(data));
	}

	function send(action, payload = {}) {
		if (socket && socket.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify({ action, payload }));
		}
	}

	connect();

	return { on, send };
})();
