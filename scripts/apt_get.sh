#!/usr/bin/env bash
# Usage: apt_get.sh <seconds> <apt-get arguments>...
#
# Runs apt-get on a CI runner, gives each attempt <seconds> and makes up to
# three. Normally `apt-get update` takes 5-10 seconds, but its tail on the
# hosted runners is long: on a single push to main it took 205 and 291 seconds
# in two build batches, and in the Wine job it printed nothing for 20 minutes,
# until the job's own timeout killed it. A second attempt on a mirror that
# answers is quicker than waiting out one that does not.
#
# `download` writes into the current directory as the calling user, which is
# what `apt-get download` wants; every other command runs under sudo. Between
# attempts dpkg finishes whatever a killed install left half-configured. When
# every `update` attempt fails, the script carries on with the indexes the image
# shipped, as an install after a failed update always could; any other command
# that fails three times fails the step.
set -euo pipefail

limit=$1
shift
command=$1

run() {
  if [ "$command" = download ]; then
    timeout --kill-after=15 "$limit" apt-get "$@" </dev/null
  else
    sudo timeout --kill-after=15 "$limit" apt-get "$@" </dev/null
  fi
}

for attempt in 1 2 3; do
  if run "$@"; then
    exit 0
  fi
  echo "::warning::apt-get $command failed or ran past ${limit}s (attempt $attempt of 3)"
  if [ "$command" = install ]; then
    sudo dpkg --configure -a </dev/null || true
  fi
done

if [ "$command" = update ]; then
  echo "::warning::apt-get update failed three times; going on with the package indexes the image has"
  exit 0
fi
exit 1
