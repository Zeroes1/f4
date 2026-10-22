#!/bin/bash
# Runs inside the terminal of the manager: sends the requests of section 6 of
# docs/FAR2L_COMPAT.md that need no key and writes what comes back to $OUT/Tnn.out.
OUT=${OUT:-/tmp/fc}
mkdir -p "$OUT"
q () { # name, request, delimiter, timeout in seconds
    printf '%b' "$2"
    r=
    IFS= read -rs -t"$4" -d"$3" r
    printf '%q\n' "$r" > "$OUT/$1.out"
}
q T01a '\e[5n' n 1
q T01b '\e[6n' R 1
q T01c '\e[c' c 1
q T03a '\e[>1u\e[?u' u 1
printf '\e[<u'
q T04a '\e[?9001$p' y 1
q T04b '\e[?2004$p' y 1
q T05a '\e_far2l1\e\\\e[5n' n 2
printf '\e_far2l0\e\\'
q T10a '\e]52;c;?\a' $'\a' 2
echo done > "$OUT/DONE"
