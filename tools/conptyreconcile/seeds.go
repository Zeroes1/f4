package main

// The seeds this project keeps re-running. A seed lands here because a real
// run once failed on it; the comment is the only record of what it caught.
// The list is deliberately portable rather than Windows-only: the probe walks
// it on Windows (`conptyreconcile -suite`) and the mock walks it everywhere
// (TestKnownSeedsAgainstTheMock), so a fix cannot rot between field runs.

type knownSeed struct {
	seed int64
	why  string
}

// NOTE ON WHAT THESE SEEDS MEAN NOW. Every entry below failed against the
// INBOX conhost of the maintainer's machine (10.0.22000.2538), because that is
// the only host this tool could measure at the time. It is not the host f4
// bundles. The failures they describe are real bugs that were found and fixed,
// so the seeds stay -- but a green run on the pinned OpenConsole does not
// confirm anything about the inbox host, and a red one would not disprove it.
// They are a regression net for the reconciler, nothing more, until they have
// been re-run against the pinned host.
var knownSeeds = []knownSeed{
	{1787985364328457600, "failed the mirror and the slice before the conhost port"},
	{1788001644056794200, "first green run after the port"},
	{1788002866976838800, "failed the mirror on exact-width chains held merged by the legacy write path; fixed by porting WriteCharsLegacy"},
	{1788003154672129800, "first green run with the reference-window stage"},
	{1788003562700467700, "green alongside the first 300-seed mock sweep"},
	{1788006508026299100, "the probe cut its own capture: a slow run had printed 22 of 151 lines when the fixed sleep expired, so the repaint pictured a mostly empty buffer; fixed by waiting for the child's marker"},
}
