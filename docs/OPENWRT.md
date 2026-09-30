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
