#!/usr/bin/env bash
# Usage: apt_get.sh <seconds> <apt-get arguments>...
#
# Runs apt-get on a CI runner with the fastest archive mirror first, and gives
# each attempt <seconds>, up to three.
#
# The hosted runners fetch packages through apt's mirror method, from the list
# in /etc/apt/apt-mirrors.txt: azure.archive.ubuntu.com, archive.ubuntu.com,
# security.ubuntu.com. apt moves on to the next mirror only when one fails, and
# the Azure mirror does not fail, it slows down: at one moment it served a
# runner 0.1 MB/s while the other two served the same file to the same runner
# at 6 MB/s, and half an hour earlier it had served 9 MB/s itself. Slowed like
# that it held qemu-user-static's update and install at 205 and 291 seconds in
# two build batches of a single push to main, and the Wine job's 109 MB install
# until the job's 20-minute limit killed it, twice. So the first call in a job
# times an index file of 1-2 MB from every listed mirror, five seconds at most
# each, and gives them priorities in order of speed. Every mirror stays on the
# list, so one that fails is still passed over the way apt always did, and apt
# checks the archive's signatures whichever mirror serves them.
#
# A transfer can still slow down after that. A fresh attempt gets a fresh
# connection, which is quicker than waiting such a transfer out; pick
# <seconds> so that three attempts still fit the job's timeout-minutes.
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

# probe_mirrors <list> <path>: "<bytes/s>\t<line>" for every mirror line,
# timing <path> on each with five seconds at most.
probe_mirrors() {
  local line url result
  while IFS= read -r line; do
    case "$line" in
      '' | '#'*) continue ;;
    esac
    url=${line%%[[:space:]]*}
    # An error page arrives fast, so only a 200 counts as speed.
    result=$(curl -s -o /dev/null --max-time 5 -w '%{http_code} %{speed_download}' "${url%/}/$2" || true)
    case "$result" in
      200\ *) printf '%s\t%s\n' "${result#200 }" "$line" ;;
      *) printf '0\t%s\n' "$line" ;;
    esac
  done < "$1"
}

# Once per job: put the fastest mirror of apt's list first. APT_MIRRORS names
# another list, for trying this out away from a runner.
rank_mirrors() {
  local list=${APT_MIRRORS:-/etc/apt/apt-mirrors.txt}
  local marker="${RUNNER_TEMP:-/tmp}/apt-mirrors-ranked"
  if [ ! -f "$list" ] || [ -e "$marker" ]; then
    return 0
  fi
  touch "$marker"

  local codename arch probe ranked
  codename=$(sed -n 's/^VERSION_CODENAME=//p' /etc/os-release 2>/dev/null || true)
  arch=$(dpkg --print-architecture)
  probe="dists/$codename/main/binary-$arch/Packages.gz"
  # Stable, so mirrors that timed the same keep the order they came in.
  ranked=$(probe_mirrors "$list" "$probe" | sort -s -t "$(printf '\t')" -k1,1 -g -r)
  case "$ranked" in
    '' | 0*)
      # Nothing answered with the file: the timing says nothing about
      # which mirror is better, so leave the list alone.
      echo "::warning::no apt mirror served $probe; keeping their order"
      return 0 ;;
  esac
  echo "apt mirrors, fastest first (bytes/s for $probe):"
  printf '%s\n' "$ranked" | sed 's/^/  /'
  # The order of the lines alone means nothing to apt: mirrors without a
  # priority are tried in random order (apt-transport-mirror(1)). So each gets
  # priority:N, lowest first, in place of any it had; other metadata stays.
  printf '%s\n' "$ranked" | cut -f2- | prioritize | sudo tee "$list" >/dev/null
}

# prioritize: number mirror lines on stdin priority:1, priority:2, ... in turn.
prioritize() {
  local n=0 line url item kept
  local -a meta
  while IFS= read -r line; do
    n=$((n + 1))
    url=${line%%[[:space:]]*}
    read -r -a meta <<<"${line#"$url"}"
    kept=""
    for item in "${meta[@]+"${meta[@]}"}"; do
      case "$item" in
        priority:*) ;;
        *) kept="$kept $item" ;;
      esac
    done
    printf '%s\tpriority:%d%s\n' "$url" "$n" "$kept"
  done
}

# Best effort: with the list as it came, apt works the way it always did.
rank_mirrors || echo "::warning::could not rank the apt mirrors; keeping their order"

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
