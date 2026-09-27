#!/bin/bash

# Builds plugins/observer's test fixture (testdata/stub/observer_stub.c) into
# a WASI-reactor .wasm with wasi-sdk. See plugins/observer/doc.go and
# testdata/stub/observer_stub.c for what the fixture is and why it exists.
#
# This is a build step, not a build of f4 itself: f4's own policy is that
# nothing gets compiled on a contributor's machine, only in CI (see
# AGENTS.md), and this script exists so quick.yml/build.yml can run it
# before `go test` the same way colorer4go's build_wasm.sh is run before its
# own tests -- it is not meant to be run by hand outside that CI step.
#
# Default path:
#   WASI_SDK_PATH=/opt/wasi-sdk
#
# Override it if necessary:
#   WASI_SDK_PATH=/path/to/wasi-sdk ./scripts/build_observer_test_wasm.sh

set -e

WASI_SDK_PATH="${WASI_SDK_PATH:-/opt/wasi-sdk}"
CLANG="$WASI_SDK_PATH/bin/clang"

if [ ! -x "$CLANG" ]; then
    echo
    echo "Error: WASI SDK clang was not found at:"
    echo "  $CLANG"
    echo
    echo "Please set WASI_SDK_PATH to your WASI SDK installation."
    echo "For example:"
    echo "  WASI_SDK_PATH=/path/to/wasi-sdk ./scripts/build_observer_test_wasm.sh"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OBSERVER_DIR="$REPO_ROOT/plugins/observer"
SRC="$OBSERVER_DIR/testdata/stub/observer_stub.c"
OUT="$OBSERVER_DIR/testdata/observer_stub.wasm"

echo "Using WASI SDK: $WASI_SDK_PATH"
echo "Building: $SRC -> $OUT"

"$CLANG" \
    -O2 \
    -mexec-model=reactor \
    -Wl,--export=LoadSubModule \
    -Wl,--export=UnloadSubModule \
    -Wl,--export=f4observer_open_storage \
    -Wl,--export=f4observer_close_storage \
    -Wl,--export=f4observer_close_count \
    -Wl,--export=f4observer_last_data_size \
    -Wl,--export=f4observer_last_data_first_byte \
    -Wl,--export=f4observer_last_progress_result \
    -Wl,--export=malloc \
    -Wl,--export=free \
    -o "$OUT" \
    "$SRC"

echo "Success!"
echo "WASM binary: $OUT"
