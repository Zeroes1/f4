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

## Open points

- Resource budget: an mc installation is a few megabytes on flash and a few
  megabytes of RSS. The current lite binary is about 40-47 MB, an order of
  magnitude above that. Whether the extra-lite profile can approach mc, and at
  which feature cost, is a decision for the owner; the numbers above are the
  baseline. RSS and start-up time are not measured yet, and neither is the mc
  baseline on the same device class.
- Extra-lite exclusions are not chosen yet. The first step is to find what the
  40 MB consist of (per-package size of the linked binary) and list the
  largest optional parts.
- CI: an OpenWrt target build with artifact name and size in the job summary,
  and a smoke check that starts the binary and drives the panel/navigation path
  without a GUI, are not added yet.
