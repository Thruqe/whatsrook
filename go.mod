module whatsrook

go 1.26.5

require (
	github.com/coder/websocket v1.8.15
	github.com/google/uuid v1.6.0
	github.com/rs/zerolog v1.35.1
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/writeas/go-strip-markdown/v2 v2.1.1
	go.mau.fi/util v0.9.12-0.20260717235539-f9ffa7eca58d
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/Thruqe/htmlbuilder v1.0.0
	github.com/beeper/argo-go v1.1.2
	github.com/lib/pq v1.12.3
	github.com/polymorfa/libsignal-protocol-go v0.2.3-0.20260806162910-a2adef2e8a11
	github.com/tcolgate/mp3 v0.0.0-20170426193717-e79c5a46d300
	modernc.org/sqlite v1.55.0
	whatsrook/cli v0.0.0-00010101000000-000000000000
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.0 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/hajimehoshi/go-mp3 v0.3.4
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/petermattis/goid v0.0.0-20260725062400-500c67a39b75 // indirect
	github.com/pion/datachannel v1.6.2
	github.com/pion/dtls/v3 v3.1.5
	github.com/pion/logging v0.2.4
	github.com/pion/opus v0.1.0
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.11.1
	github.com/pion/transport/v4 v4.0.2 // indirect
	golang.org/x/crypto v0.54.0
	golang.org/x/exp v0.0.0-20260727155853-b88d891fe743 // indirect
	golang.org/x/net v0.57.0
	golang.org/x/sync v0.22.0
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/nonibytes/ffgo => github.com/obinnaokechukwu/ffgo v0.1.1

replace whatsrook/cli => ./cli
