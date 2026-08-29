# conptydump — capture, don't decide

This tool answers nothing. It records what a ConPTY emits, byte for byte, with
a timestamp on every chunk and a marked line wherever it called
`ResizePseudoConsole`. The analysis happens afterwards, off the file.

That is the whole design, and it is a reaction to how the probes before it
failed. Each of them decided *while running* whether a phase had finished —
by waiting for a marker, or for the stream to fall silent — and each of those
decisions was wrong in a way that produced a confident zero: a reflow column
of `0B` that looked like a ConPTY finding and was a bug in the waiting. A dump
cannot fail that way. The bytes are either in the file or they are not, and a
mistake in reading it costs a re-read instead of another run on the tester's
machine.

## What it does, in order

1. Two `CreatePipe` calls with NULL security attributes.
2. `CreatePseudoConsole`, then **close both pty-side handles immediately** —
   EchoCon.cpp says they are dup'ed into the ConHost.
3. A reader goroutine that loops on `ReadFile` until it fails. No timeouts, no
   conditions, no idea what it is reading.
4. `CreateProcessW` with `EXTENDED_STARTUPINFO_PRESENT` and nothing else, an
   attribute list of one `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` entry, and
   `STARTF_USESTDHANDLES` with three `INVALID_HANDLE_VALUE` so the child cannot
   inherit our stdio (wezterm's `psuedocon.rs` does this and explains why).
5. Sleep, resize to width−1, sleep, resize to 4000, sleep, restore, sleep.
   Fixed delays. Nothing is waited for.
6. Close, with `ClosePseudoConsole` on its own goroutine because it can block
   until the host exits.

Every ConPTY call above matches Microsoft's `EchoCon` sample and WezTerm's
`psuedocon.rs` where the two agree.

## Running it

```
conptydump.exe                          # 120x2000, writes conptydump-2000.txt
conptydump.exe -height 32000
conptydump.exe -height 500 -step 3s
```

Takes about ten seconds. Send the `.txt`.

## The file

```
@@       0ms EVENT start width=120 height=2000 ...
@@      12ms EVENT CreatePseudoConsole ok, hpc=0x...
@@      45ms CHUNK 61 bytes (total 61)
\e[?25l\e[2J\e[m\e[H...
@@    2011ms EVENT RESIZE -> 119x2000 (narrow by one)
@@    2013ms EVENT ResizePseudoConsole(119x2000) ok
@@    2049ms CHUNK 8192 bytes (total 12480)
...
```

Escaping keeps it a text file: `\e` for ESC, `\r`, `\n` (with a real newline
after it so the file stays readable), `\xNN` for other control bytes,
everything printable as itself. Nothing is interpreted or normalised.

Reading it needs no tooling: the chunk after a `RESIZE` event is what conhost
sent in response, its byte count says whether the whole history was
re-transmitted, and searching for `~F000001~` says whether the *oldest* line
came back or only the tail.
