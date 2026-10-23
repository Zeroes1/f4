#!/bin/bash
# Runs inside the terminal of the manager: sets up one probe that needs keys,
# says it is ready, reads what the host sends for 4 seconds and writes it to
# $OUT/<name>.out. The driver presses the keys.
OUT=${OUT:-/tmp/fc}
mkdir -p "$OUT"
name=$1
case "$name" in
T02a) printf '\e[?2004h' ;;
T02b) printf '\e[?2004l' ;;
T03b) printf '\e[>1u' ;;
T04c) printf '\e[?9001h' ;;
T07b) ;; # the modes were set and dropped by the command typed before
esac
# raw mode: the terminal driver must not turn CR into LF or hold the input until a line is complete
stty raw -echo
touch "$OUT/$name.ready"
# Read what the host sends for 4 seconds of quiet; the driver shows it like cat -v.
acc=
while IFS= read -rsn1 -t4 -d '' c; do acc+=$c; done
printf '%s' "$acc" > "$OUT/$name.out"
case "$name" in
T03b) printf '\e[<u' ;;
T04c) printf '\e[?9001l' ;;
T02a) printf '\e[?2004l' ;;
esac
stty sane
touch "$OUT/$name.done"
