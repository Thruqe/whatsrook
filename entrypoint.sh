#!/bin/sh

HAS_EXPLICIT_FLAG=false
RAW_PHONE=""

for arg in "$@"; do
  case "$arg" in
    -s|--session|--session=*|-s=*)
      HAS_EXPLICIT_FLAG=true
      break
      ;;
    *)
      clean_num=$(echo "$arg" | tr -d '+')
      case "$clean_num" in
        ''|*[!0-9]*) ;;
        *)
          if [ ${#clean_num} -ge 7 ] && [ ${#clean_num} -le 15 ]; then
            RAW_PHONE="$arg"
          fi
          ;;
      esac
      ;;
  esac
done

ARGS=""

if [ "$HAS_EXPLICIT_FLAG" = "false" ]; then
  if [ -n "$RAW_PHONE" ]; then
    SESSION="$RAW_PHONE"
    ARGS="-s $RAW_PHONE"
  elif [ -n "$SESSION" ]; then
    ARGS="-s $SESSION"
  fi
fi

if [ -z "$SESSION" ] && [ "$HAS_EXPLICIT_FLAG" = "false" ]; then
  echo "Error: SESSION environment variable or --session / -s argument is required."
  exit 1
fi

if [ "$PAIR" = "true" ] || [ "$PAIR" = "1" ]; then
  ARGS="$ARGS -p"
fi

if [ -n "$CLIENT" ]; then
  ARGS="$ARGS -c $CLIENT"
fi

if [ "$QRCODE" = "true" ] || [ "$QRCODE" = "1" ]; then
  ARGS="$ARGS -q"
fi

if [ "$LOGOUT" = "true" ] || [ "$LOGOUT" = "1" ]; then
  ARGS="$ARGS -l"
fi

if [ "$VERBOSE" = "true" ] || [ "$VERBOSE" = "1" ]; then
  ARGS="$ARGS -v"
fi

if [ -f "./bin/whatsrook" ]; then
  exec ./bin/whatsrook $ARGS "$@"
elif [ -f "./whatsrook" ]; then
  exec ./whatsrook $ARGS "$@"
else
  exec go run ./cli $ARGS "$@"
fi