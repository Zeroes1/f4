#!/usr/bin/env bash
# Usage: apt_get.sh <seconds> <apt-get arguments>...
#
# Runs apt-get on a CI runner from the archive mirror that is fastest for what
# it is about to download, and gives each attempt <seconds>, up to three.
#
# The hosted runners fetch packages through apt's mirror method, from
# /etc/apt/apt-mirrors.txt: azure.archive.ubuntu.com priority:1,
# archive.ubuntu.com priority:2, security.ubuntu.com priority:3. apt leaves a
# mirror only when it fails, and the Azure mirror does not fail, it slows down,
# and only for some files. In one run it served the index Packages.gz at
# 157 MB/s and, a minute later, libwine's 105 MB at 98 kB/s; the other two
# mirrors serve such a file at about 6 MB/s. Slowed like that it held
# qemu-user-static's install at 205 and 291 seconds in two build batches of a
# single push to main, and the Wine job's install until the job's 20-minute
# limit killed it, more than once.
#
# So the first install or download in a job asks apt which files it would
# fetch, takes the largest, and times its first 4 MB on every listed mirror,
# five seconds at most each; then it gives the mirrors priorities in order of
# that speed. An index file would not do: the mirror serves those from a cache,
# which is how the slow one came out fastest when this timed Packages.gz. Every
# mirror stays on the list, so one that fails is still passed over the way apt
# always did, and apt checks the archive's signatures whichever mirror serves
# them. An update needs none of this; indexes come fast from all three.
#
# A transfer can still slow down later. When an attempt runs past <seconds>,
# the mirror that came first goes to the end of the list and the next attempt
# starts on another one. Pick <seconds> so that three attempts still fit the
# job's timeout-minutes.
#
# A connection that stops sending altogether fails after 30 seconds instead of
# apt's default, and apt retries that file itself. `download` writes into the
# current directory as the calling user, which is what `apt-get download`
# wants; every other command runs under sudo. Between attempts dpkg finishes
# whatever a killed install left half-configured.
#
# The limit is there to cut a stall short, never to fail a step that would
# have got there by waiting. So after three timed-out attempts an install or
# download gets one more with no limit, exactly what the workflows ran before
# this script, and fails only if that fails. An update that fails three times
# is given up instead: the step goes on with the indexes the image shipped, as
# an install after a failed update always could.
#
# APT_MIRRORS names another mirror list, for trying this out away from a runner.
set -euo pipefail

limit=$1
shift
command=$1

mirror_list=${APT_MIRRORS:-/etc/apt/apt-mirrors.txt}

as_root=sudo
if [ "$command" = download ]; then
  as_root=
fi

# largest_pool_path <apt-get arguments>...: the pool/ path of the largest file
# the command would download, or nothing when it would download nothing.
largest_pool_path() {
  # Each line: 'URI' file size hash. The URI may name the mirror list rather
  # than a mirror, but the path from pool/ on is the same on every mirror.
  $as_root apt-get --print-uris -qq "$@" </dev/null 2>/dev/null |
    awk '{ uri = $1; gsub(/\047/, "", uri); i = index(uri, "/pool/")
           if (i && $3 + 0 > max) { max = $3 + 0; path = substr(uri, i + 1) } }
         END { if (path != "") print path }'
}

# probe_mirrors <path>: "<bytes/s>\t<line>" for every mirror line, timing the
# first 4 MB of <path> on each with five seconds at most.
probe_mirrors() {
  local line url result
  while IFS= read -r line; do
    case "$line" in
      '' | '#'*) continue ;;
    esac
    url=${line%%[[:space:]]*}
    # An error page arrives fast, so only the file itself counts as speed.
    result=$(curl -s -o /dev/null -r 0-4194303 --max-time 5 \
      -w '%{http_code} %{speed_download}' "${url%/}/$1" || true)
    case "$result" in
      200\ * | 206\ *) printf '%s\t%s\n' "${result#* }" "$line" ;;
      *) printf '0\t%s\n' "$line" ;;
    esac
  done < "$mirror_list"
}

# prioritize: number mirror lines on stdin priority:1, priority:2, ... in turn,
# in place of any priority they had; other metadata stays. The order of the
# lines alone means nothing to apt: without priorities it contacts mirrors in
# random order (apt-transport-mirror(1)).
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

# by_priority: the mirror lines, lowest priority first; a line without one
# sorts last, as apt treats it.
by_priority() {
  local line priority
  while IFS= read -r line; do
    case "$line" in
      '' | '#'*) continue ;;
    esac
    priority=$(sed -n 's/.*[[:space:]]priority:\([0-9][0-9]*\).*/\1/p' <<<"$line")
    printf '%s\t%s\n' "${priority:-999999}" "$line"
  done < "$mirror_list" | sort -s -t "$(printf '\t')" -k1,1 -n | cut -f2-
}

# Once per job, before the first install or download: the mirror that is
# fastest for the largest file to fetch comes first.
rank_mirrors() {
  local marker="${RUNNER_TEMP:-/tmp}/apt-mirrors-ranked"
  if [ ! -f "$mirror_list" ] || [ -e "$marker" ]; then
    return 0
  fi
  local path ranked
  path=$(largest_pool_path "$@")
  if [ -z "$path" ]; then
    return 0 # nothing to download, so nothing to time; a later call will
  fi
  touch "$marker"
  # Stable, so mirrors that timed the same keep the order they came in.
  ranked=$(probe_mirrors "$path" | sort -s -t "$(printf '\t')" -k1,1 -g -r)
  case "$ranked" in
    '' | 0*)
      # Nothing answered with the file: the timing says nothing about which
      # mirror is better, so leave the list alone.
      echo "::warning::no apt mirror served $path; keeping their order"
      return 0 ;;
  esac
  echo "apt mirrors, fastest first (bytes/s for the first 4 MB of $path):"
  printf '%s\n' "$ranked" | sed 's/^/  /'
  printf '%s\n' "$ranked" | cut -f2- | prioritize | write_mirror_list
}

# After a timed-out attempt: the mirror that came first goes last.
demote_first_mirror() {
  if [ ! -f "$mirror_list" ]; then
    return 0
  fi
  local ordered
  ordered=$(by_priority)
  if [ -z "$ordered" ]; then
    return 0
  fi
  { sed -n '2,$p' <<<"$ordered"; sed -n '1p' <<<"$ordered"; } | prioritize | write_mirror_list
  echo "apt mirrors for the next attempt: $(by_priority | sed 's/[[:space:]].*//' | paste -sd ' ' -)"
}

# write_mirror_list: replace the mirror list, which root owns, with stdin.
write_mirror_list() {
  sudo tee -- "$mirror_list" >/dev/null
}

# run limited|unlimited <apt-get arguments>...
run() {
  local mode=$1
  shift
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

fetches=no
case "$command" in
  install | download) fetches=yes ;;
esac

# Best effort: with the list as it came, apt works the way it always did.
if [ "$fetches" = yes ]; then
  rank_mirrors "$@" || echo "::warning::could not rank the apt mirrors; keeping their order"
fi

for attempt in 1 2 3; do
  if run limited "$@"; then
    exit 0
  fi
  echo "::warning::apt-get $command failed or ran past ${limit}s (attempt $attempt of 3)"
  finish_install
  if [ "$fetches" = yes ]; then
    demote_first_mirror || true
  fi
done

if [ "$command" = update ]; then
  echo "::warning::apt-get update failed three times; going on with the package indexes the image has"
  exit 0
fi
echo "::warning::apt-get $command: one more attempt, with no time limit"
run unlimited "$@"
