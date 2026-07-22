#!/bin/sh

if [ -z "$SESSION" ]; then
  echo "Error: SESSION environment variable is required."
  exit 1
fi

ARGS="--session $SESSION"

if [ "$PAIR" = "true" ]; then
  ARGS="$ARGS --pair"
fi

if [ -n "$PORT" ]; then
  ARGS="$ARGS --port $PORT"
fi

if [ -n "$AUTH_DIR" ]; then
  ARGS="$ARGS --auth-dir $AUTH_DIR"
fi

if [ -n "$CLIENT" ]; then
  ARGS="$ARGS --client $CLIENT"
fi

if [ "$QRCODE" = "true" ]; then
  ARGS="$ARGS --qrcode"
fi

if [ "$LOGOUT" = "true" ]; then
  ARGS="$ARGS --logout"
fi

if [ "$DEBUG" = "true" ]; then
  ARGS="$ARGS --debug"
fi

if [ "$VERBOSE" = "true" ]; then
  ARGS="$ARGS --verbose"
fi

if [ "$DEV" = "true" ]; then
  ARGS="$ARGS --dev"
fi

exec ./whatsrook $ARGS "$@"