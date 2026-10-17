#!/usr/bin/env bash
# Drag-out acceptance probe for the Wine GUI backend (unxed/f4#566).
#
# Runs f4.exe --gui=win32 under Wine on the current X display (the Wine job
# starts it inside xvfb-run), then drags the only file of its panel onto
# tools/xdndtarget, a bare XDND drop target on the same display. The target's
# exit code is the verdict (0: the exact path arrived as text/uri-list).
#
# The panel geometry is not known in advance, so the probe tries one row after
# another below the window's top edge and stops at the first drop that lands.
#
# usage: wine-xdnd-drag.sh <build dir with f4.exe and xdndtarget> <log dir>
# env:   WINE  the wine loader
set -u

build=${1:?build dir}
logs=${2:?log dir}
: "${WINE:?}"

mkdir -p "$logs" "$build/dragdir"
printf 'xdnd\n' > "$build/dragdir/dragme.txt"
want="$build/dragdir/dragme.txt"

cd "$build/dragdir" || exit 2
"$WINE" "$build/f4.exe" --debug --dump-screen-after 10 --gui=win32 > "$logs/f4.out" 2>&1 &
f4pid=$!
trap 'kill "$f4pid" 2>/dev/null || true' EXIT
sleep 12

# The f4 window is the largest visible window that is not the whole screen.
best=0
for id in $(xdotool search --onlyvisible --name . 2>/dev/null); do
  eval "$(xdotool getwindowgeometry --shell "$id" 2>/dev/null)" || continue
  area=$((WIDTH * HEIGHT))
  if [ "$WIDTH" -lt 1280 ] && [ "$area" -gt "$best" ]; then
    best=$area wx=$X wy=$Y ww=$WIDTH wh=$HEIGHT wid=$id
  fi
done
if [ "$best" -eq 0 ]; then
  echo "no visible f4 window" | tee "$logs/verdict.txt"
  exit 1
fi
echo "f4 window $wid at ${wx},${wy} size ${ww}x${wh}" | tee "$logs/window.txt"

# The text dump of the screen f4 wrote after 10 s: the row/column layout the
# probe's guesses below have to hit.
find "$build" -maxdepth 2 -name vtui.screen.log -exec cp -t "$logs" {} + 2>/dev/null || true

rows=25
for k in 2 3 4 5 6 7 8; do
  "$build/xdndtarget" -x 1000 -y 700 -w 250 -h 250 -timeout 8s \
    -want-path "$want" -out "$logs/uri-$k.txt" > "$logs/target-$k.out" 2>&1 &
  tpid=$!
  for _ in $(seq 1 50); do
    grep -q '^READY' "$logs/target-$k.out" 2>/dev/null && break
    sleep 0.1
  done
  sx=$((wx + ww / 4))
  sy=$((wy + (2 * k + 1) * wh / (2 * rows)))
  xdotool mousemove "$sx" "$sy" click 1 sleep 0.3 mousedown 1 sleep 0.3
  for step in 1 2 3 4 5 6 7 8 9 10; do
    xdotool mousemove $((sx + (1100 - sx) * step / 10)) $((sy + (800 - sy) * step / 10)) sleep 0.1
  done
  xdotool sleep 0.3 mouseup 1
  rc=0
  wait "$tpid" || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "drop landed from row $k (${sx},${sy})" | tee "$logs/verdict.txt"
    exit 0
  fi
  echo "row $k (${sx},${sy}): target exit $rc" >> "$logs/attempts.txt"
done
echo "no drop landed" | tee "$logs/verdict.txt"
exit 1
