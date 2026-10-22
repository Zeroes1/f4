//go:build windows

package unpack

// sudoAvailable is false on Windows: nothing is unpacked through the elevated
// dispatcher of f4#1768 there. The updater elevates the whole archive itself,
// through its UAC helper, when a step is refused (update.runElevated); a step
// handed to the dispatcher instead fails with the dispatcher's error, not the
// refusal, and that helper is never started. And the refusal every update
// meets -- f4.exe.old, the program running the update, cannot be deleted --
// is one no administrator can lift. Asking for elevation there held
// `f4 --update` for two minutes, waiting for an answer to a question no
// window showed (#2172).
func sudoAvailable() bool { return false }
