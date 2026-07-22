# WhatsRook WebSocket Client Examples

This directory contains working examples showing how to connect to the **WhatsRook** WebSocket gateway (`ws://localhost:8080/ws`), stream real-time WhatsApp events, decode incoming payloads, and issue control commands.

WhatsRook uses **Protocol Buffers (`protobuf`)** for all bidirectional WebSocket communication over binary frames using `proto/ws.proto`.

## How to Run and Test

Testing requires running the **WhatsRook server daemon** in one terminal and the **example client** in another.

### Step 1: Start the WhatsRook Server (Terminal 1)

Make sure the WhatsRook daemon is running and listening on your designated port (e.g. `8080`):

```bash
./whatsrook --session your_phone_number --port 8080 --dev --verbose
```

_(Or start via `go run . --session your_phone_number --port 8080 --dev`)_

### Step 2: Run the Example Client (Terminal 2)

In a separate terminal window, run the Go WebSocket client:

```bash
go run ./example -url ws://localhost:8080/ws
```

### Configuration Options ([`config.go`](./example/config.go))

The example client accepts CLI flags configured in `config.go`:

- `-pair`: Enable pair code request flow log.
- `-qrcode`: Enable QR code event log.
- `-logout`: Automatically transmit a Protobuf `CONTROL_TYPE_LOGOUT` command after **30 seconds** of connection.

Example with automated 30s logout:

```bash
go run ./example -url ws://localhost:8080/ws -logout
```

## Message Payload Contracts

### Inbound Events (`EventFrame`)

Incoming binary event frames received from WhatsRook:

- `EVENT_TYPE_CONNECTED`: WhatsApp daemon connected.
- `EVENT_TYPE_INCOMING_MESSAGE`: Incoming text or media message (`IncomingMessagePayload`).
- `EVENT_TYPE_INCOMING_CALL`: Incoming WhatsApp audio/video call offer (`IncomingCallPayload`).
- `EVENT_TYPE_PAIR_CODE` / `EVENT_TYPE_PAIR_QR`: Authentication codes issued during setup.
- `EVENT_TYPE_STATUS`: Connection and session metadata (`StatusPayload`).
- `EVENT_TYPE_ACK`: Acknowledgment frame returned for control requests.

### Outbound Control Messages (`ControlFrame`)

Supported control commands sent to WhatsRook:

- `CONTROL_TYPE_SEND_MESSAGE`: Send a text message (or quote a reply).
- `CONTROL_TYPE_SEND_REACTION`: React to a message with an emoji.
- `CONTROL_TYPE_EDIT_MESSAGE`: Edit a previously sent message.
- `CONTROL_TYPE_REVOKE_MESSAGE`: Delete/revoke a message for everyone.
- `CONTROL_TYPE_GET_STATUS`: Query bot connection status.
- `CONTROL_TYPE_DISCONNECT` / `CONTROL_TYPE_LOGOUT`: Manage WhatsApp session state.

## Protocol Buffer Schema

Refer to [`proto/ws.proto`](../proto/ws.proto) for the full Protobuf message specification.
