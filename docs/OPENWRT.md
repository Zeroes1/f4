# Extra-lite profile and OpenWrt targets (f4#1671)

Status: design notes and measurements. The existing lite profile
(`go build -tags lite,vtui_noebiten,vtui_nogogpu`, see "Lite build" in the
README) is the starting point; nothing here changes the regular or the lite
build. This file records what was measured and what is still open, so that the
extra-lite profile and the OpenWrt artifacts are built from facts.

## Toolchain assumptions

- Go's own toolchain, `CGO_ENABLED=0`, `-trimpath -ldflags "-s -w"`. The binary
  is static, so no libc (musl on OpenWrt) is linked or required at run time.
- OpenWrt is `GOOS=linux`. Only the GOARCH and its floating-point/ABI variable
  differ per target.

## Target matrix (candidate, to be confirmed on real OpenWrt SDK targets)

| OpenWrt architecture family | GOOS/GOARCH | Variable |
| --- | --- | --- |
| `mips_24kc` (big endian, ath79 and others) | linux/mips | `GOMIPS=softfloat` |
| `mipsel_24kc`, `mipsel_74kc`, `mipsel_mips32` (ramips, ath79) | linux/mipsle | `GOMIPS=softfloat` |
| `arm_arm926ej-s`, `arm_xscale` (ARMv5) | linux/arm | `GOARM=5` |
| `arm_cortex-a7`, `-a9`, `-a15` (ARMv7) | linux/arm | `GOARM=7` |
| `aarch64_*` | linux/arm64 | none |
| `x86_64` | linux/amd64 | none |
| `i386_pentium4`, `i386_pentium-mmx` | linux/386 | `GO386=softfloat` |
| `riscv64_riscv64` | linux/riscv64 | none |
| `mips64_octeonplus` and other 64-bit MIPS | linux/mips64 | none |

The mapping comes from the OpenWrt architecture names and Go's port list; it
has not yet been checked by running a binary on each family, so it is a
candidate list, not a promise of support.

## Measured lite binary sizes

Stripped, `-trimpath`, current `lunobot/staging`
(sandbox run https://github.com/unxed/f4/actions/runs/36620252583, all nine
targets build):

| Target | Size, bytes |
| --- | --- |
| linux/riscv64 | 39649442 |
| linux/arm (GOARM=7) | 40501410 |
| linux/arm (GOARM=5) | 40632482 |
| linux/386 (softfloat) | 40886434 |
| linux/arm64 | 44237065 |
| linux/mips, linux/mipsle (softfloat) | 45023425 |
| linux/mips64 | 45809826 |
| linux/amd64 | 47218953 |

## Extra-lite profile

Built with `-tags lite,extralite,vtui_noebiten,vtui_nogogpu`. `extralite`
implies everything the lite profile leaves out and adds its own exclusions.
Current exclusions:

- Embedded translations other than English (`internal/i18n/langfs_extralite.go`,
  about 5.4 MB). A translation is still loaded from the language directories
  on disk, so the feature is not lost, only the copy inside the binary.
- `golang.org/x/text/collate` (about 1.2 MB): file names are ordered by
  `internal/panel/namecompare_extralite.go`, which follows the root collation
  for white space, punctuation, digits and letters, but does not fold accented
  Latin letters onto their base letters.
- The East Asian code pages (Shift JIS, ISO-2022-JP, EUC-JP, EUC-KR, GBK,
  HZ-GB-2312, GB18030, Big5; `vfs/codepages_nocjk.go`, about 0.6 MB). UTF-8,
  UTF-16 and the single-byte code pages remain.

## Measurements

Linux amd64, stripped, `-trimpath`; start-up and peak RSS from
`scripts/openwrt_smoke.py` (pseudo-terminal, panel listing plus a typed `cd`),
GitHub ubuntu-latest runner:

| | binary, bytes | listing shown | peak RSS |
| --- | --- | --- | --- |
| f4 lite | 47 534 345 | about 0.9 s | about 38 MB |
| f4 extralite | 42 111 241 | about 0.75 s | about 39 MB |
| mc 4.8.30 (Ubuntu package) | 1 140 880 (package 1 555 KB installed) | about 0.2 s | about 11 MB |

Cross-built sizes, bytes (lite / extralite): mipsle-softfloat 45 351 105 /
39 911 617, arm v7 40 829 090 / 35 389 602, arm64 44 499 209 / 39 125 257,
amd64 47 534 345 / 42 111 241. All eight build; the amd64 pair passes the
smoke check (panel listing and a typed `cd`). RSS and start-up were measured
on a GitHub runner, not on router hardware; the extra-lite profile saves
binary size, not memory, since the embedded translations are read lazily.

The `openwrt` workflow prints these numbers for every run.

Compressed sizes, which are what an `.ipk` or a squashfs image carries
(bytes, `gzip -9` / `xz -9e`), lite and extralite, after the collation and
East Asian code page slice:

| target | lite gz | lite xz | extralite gz | extralite xz |
| --- | --- | --- | --- | --- |
| mipsle-softfloat | 14 283 600 | 8 952 380 | 11 925 423 | 7 947 420 |
| arm v7 | 14 858 962 | 9 669 036 | 12 505 721 | 8 661 440 |
| arm64 | 15 364 297 | 10 122 452 | 13 007 442 | 9 118 464 |
| amd64 | 16 739 968 | 11 676 116 | 14 374 672 | 10 658 996 |

(Raw extralite sizes at that point: mipsle 38 600 897, arm 34 013 346, arm64
37 880 073, amd64 40 907 017.)

What is not reachable against mc: a statically linked Go binary carries its
own runtime, garbage collector, TLS/crypto and reflection tables, so it cannot
come near mc's roughly 1 MB executable plus shared libraries; the floor found
so far is about 8 MB compressed. Resident memory of a Go program is likewise
above mc's. What extralite keeps is the function set the panels, viewer and
editor give; what it gives up is listed under "Extra-lite profile" above and
grows with each slice.

Where the lite binary's 47 MB are: `.text` 17.4 MB, `.rodata` 13.1 MB,
`.gopclntab` 13.6 MB. The 32 MiB `crypto/internal/fips140/drbg.memory` that
`go tool nm` lists first is a zero-filled `.bss` buffer of Go's own FIPS pool:
it is not in the file and does not count towards the binary size.

Largest optional parts by linked size (candidates for later slices, each only
if it can go without taking a feature away): embedded translations 5.7 MB
(done above), `golang.org/x/text/collate` 1.25 MB, wazero about 1 MB (wasm
plugins), `github.com/yuin/gopher-lua` 0.29 MB (Lua plugins), `net/http` and
`crypto/tls` about 0.6 MB together, `plugins/mediainfo` 0.27 MB,
`golang.org/x/text/encoding` CJK tables about 0.6 MB, `ebitengine/purego`
0.9 MB.

## Decision

Decided independently, without asking the owner: the extra-lite profile does
not try to match mc's size, which a Go binary with an editor, a viewer,
plugins and 25 languages cannot reach; it removes what can go without taking a
function away, biggest first, and every step is measured by the workflow
above. Removing whole features (Lua, wasm plugins) is left for a separate
decision when it comes to that.

## Open points

- The mc baseline was measured on the amd64 runner only; a comparison on the
  target router hardware is not done.
- Further exclusions (the list above) are separate slices.
