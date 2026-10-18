package terminal

import "fmt"

// ManagedForegroundCommand wraps sqCmd -- a command already shell-quoted for
// eval, e.g. via ShellSingleQuote -- in the OSC 133 C/D markers f4 needs in
// order to know when a foreground command run from its own command line has
// finished. See internal/panel/frame.go's composition of fullWireCmd for the
// caller (the "managed foreground command" branch), and
// managed_exec_test.go for what a job-control stop (Ctrl+Z / SIGTSTP) does
// to the D marker printed here -- it never runs, which is the mechanism
// behind f4 #1603's stuck-forever terminal.
func ManagedForegroundCommand(sqCmd string) string {
	return fmt.Sprintf("{ trap \"printf ''\" INT; printf \"\\033]133;C\\007\"; eval %s ; FARVTRESULT=$?; printf \"\\033]133;D\\007\"; trap - INT; (exit $FARVTRESULT); }", sqCmd)
}

// ManagedForegroundCommandInDirectory runs the managed command only after a
// directory change succeeded. A plain `cd dir && ManagedForegroundCommand`
// is not sufficient: when the directory needs sudo, the unprivileged shell
// rejects cd and short-circuits the OSC wrapper, leaving f4 waiting forever
// for its completion marker (f4#1255).
//
// sqPath and sqCmd are already shell-quoted for the POSIX shell. The failure
// arm emits the same C/D markers as a managed command, then preserves cd's
// exit status, so the terminal returns to the panels instead of hanging.
func ManagedForegroundCommandInDirectory(sqPath, sqCmd string) string {
	return fmt.Sprintf("set +H; if cd %s; then %s; else FARVTRESULT=$?; printf \"\\033]133;C\\007\\033]133;D\\007\"; (exit $FARVTRESULT); fi", sqPath, ManagedForegroundCommand(sqCmd))
}

// ManagedForegroundCommandInElevatedDirectory is ManagedForegroundCommandInDirectory
// for a directory the panel reached through f4's sudo helper (f4#1255): the
// user already authenticated for it, so a command typed there must run there
// instead of dying on the unprivileged shell's refused cd.
//
// When the plain cd is refused, the command is run by sudo -- the same way a
// user would type it -- as `cd -- <dir> && eval <cmd>`, between the same C/D
// markers a managed command prints, so Ctrl+C reaches sudo and not the
// wrapper. sudo asks for the password in the terminal itself when its own
// timestamp has expired, which keeps the credential out of f4; the command
// is sudo's, never an unprivileged retry. Without sudo in PATH the wrapper
// behaves exactly as ManagedForegroundCommandInDirectory does.
//
// sqPath and sqCmd are already shell-quoted for the POSIX shell.
func ManagedForegroundCommandInElevatedDirectory(sqPath, sqCmd string) string {
	elevated := fmt.Sprintf("{ trap \"printf ''\" INT; printf \"\\033]133;C\\007\"; sudo -- sh -c 'cd -- \"$1\" && eval \"$2\"' f4-sudo %s %s; FARVTRESULT=$?; printf \"\\033]133;D\\007\"; trap - INT; }", sqPath, sqCmd)
	return fmt.Sprintf("set +H; if cd %s; then %s; else FARVTRESULT=$?; if command -v sudo >/dev/null 2>&1; then %s; else printf \"\\033]133;C\\007\\033]133;D\\007\"; fi; (exit $FARVTRESULT); fi", sqPath, ManagedForegroundCommand(sqCmd), elevated)
}
