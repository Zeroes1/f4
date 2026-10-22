#!/usr/bin/env bash
# Usage: apt_get.sh <seconds> <apt-get arguments>...
#
# Runs apt-get on a CI runner, gives each attempt <seconds> and makes up to
# three. The archive mirror the hosted runners use is usually quick -- an
# update in 5-10 seconds, Wine's 109 MB in about 12 -- but now and then a
# transfer slows to a trickle that never quite stops, so neither apt's own
# timeout nor anything in the log notices. On a single push to main, update
# and install of qemu-user-static together took 205 and 291 seconds in two
# build batches, and the Wine job's install twice ran until the job's 20-minute
# limit killed it. A fresh attempt
# gets a fresh connection, which is quicker than waiting such a transfer out.
# Pick <seconds> so that three attempts still fit the job's timeout-minutes.
#
# A connection that stops sending altogether fails after 30 seconds instead of
# apt's default, and apt retries that file itself. `download` writes into the
# current directory as the calling user, which is what `apt-get download`
# wants; every other command runs under sudo. Between attempts dpkg finishes
# whatever a killed install left half-configured.
#
# The limit is there to cut a stall short, never to fail a step that would
# have got there by waiting: a stall can outlast all three attempts (one
# runner could not download qemu-user-static's 15 MB for six minutes running).
# So after three timed-out attempts an install or download gets one more with
# no limit, exactly what the workflows ran before this script, and fails only
# if that fails. An update that fails three times is given up instead: the
# step goes on with the indexes the image shipped, as an install after a
# failed update always could.
set -euo pipefail

limit=$1
shift
command=$1

# run limited|unlimited <apt-get arguments>...
run() {
  local mode=$1
  shift
  local as_root=sudo
  if [ "$command" = download ]; then
    as_root=
  fi
  if [ "$mode" = limited ]; then
    $as_root timeout --kill-after=15 "$limit" apt-get -o Acquire::http::Timeout=30 "$@" </dev/null
  else
    $as_root apt-get -o Acquire::http::Timeout=30 "$@" </dev/null
  fi
}

finish_install() {
  if [ "$command" = install ]; then
    sudo dpkg --configure -a </dev/null || true
  fi
}

for attempt in 1 2 3; do
  if run limited "$@"; then
    exit 0
  fi
  echo "::warning::apt-get $command failed or ran past ${limit}s (attempt $attempt of 3)"
  finish_install
done

if [ "$command" = update ]; then
  echo "::warning::apt-get update failed three times; going on with the package indexes the image has"
  exit 0
fi
echo "::warning::apt-get $command: one more attempt, with no time limit"
run unlimited "$@"
