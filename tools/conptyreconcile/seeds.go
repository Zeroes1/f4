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

var knownSeeds = []knownSeed{
	{1787985364328457600, "failed the mirror and the slice before the conhost port"},
	{1788001644056794200, "first green run after the port"},
	{1788002866976838800, "failed the mirror on exact-width chains held merged by the legacy write path; fixed by porting WriteCharsLegacy"},
	{1788003154672129800, "first green run with the reference-window stage"},
	{1788003562700467700, "green alongside the first 300-seed mock sweep"},
}
