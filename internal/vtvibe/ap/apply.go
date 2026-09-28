package ap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Status is the outcome of an Apply call.
type Status string

const (
	StatusSuccess Status = "SUCCESS"
	StatusPartial Status = "PARTIAL"
	StatusFailed  Status = "FAILED"
)

// Options controls how a patch is applied.
type Options struct {
	// Strict enables Strict Mode (§3.0): the whole patch is one atomic
	// transaction (any failure aborts with nothing written), no tolerant
	// heuristics run, and no afailed.ap/afailed.md is produced on failure
	// beyond the single fatal error.
	Strict bool
	// Silent suppresses the human-readable progress/warning text that
	// would otherwise go to Out (successes, "[TOLERANT] ..." notes,
	// idempotency skips, the structural-balance warning).
	Silent bool
	// DryRun computes the same write plan (and, on partial failure, the
	// same afailed.ap/afailed.md as a real run) but never touches
	// projectDir: the reference's --dry-run (§ CLI) skips only the OS
	// write phase, nothing about validation or reporting. When !Silent,
	// Out gets one line per planned change instead of the write actually
	// happening.
	DryRun bool
	// Out receives progress/warning text when !Silent. Defaults to
	// io.Discard.
	Out io.Writer
}

// Result is the outcome of Apply, mirroring the reference's returned dict.
type Result struct {
	Status Status
	// Error is set for StatusFailed.
	Error *AppError
	// FilePath/HasFilePath identify which file a StatusFailed error
	// happened in, when applicable (a patch-level parse error has none).
	FilePath    string
	HasFilePath bool
	// ModIdx/HasModIdx identify which modification (0-based) failed, for
	// per-modification StatusFailed errors in strict mode.
	ModIdx    int
	HasModIdx bool
	// FailedFiles lists the files with at least one failed modification,
	// for StatusPartial (tolerant mode).
	FailedFiles []string
}

// Apply applies the ap-format patch at patchFile to the tree rooted at
// projectDir, per §3 of the specification.
func Apply(patchFile, projectDir string, opts Options) *Result {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	e := &engine{strict: opts.Strict, silent: opts.Silent, dryRun: opts.DryRun, out: out, projectDir: projectDir}
	e.afailedMDPath = filepath.Join(projectDir, "afailed.md")

	patchBytes, _ := os.ReadFile(patchFile)
	e.patchContent = string(patchBytes)

	afailedAPPath := filepath.Join(projectDir, "afailed.ap")
	if !e.strict {
		if pathExists(afailedAPPath) {
			return e.fatal(StatusFailed, "", false, 0, false, &AppError{
				Code:    ErrAfailedExists,
				Message: fmt.Sprintf("afailed.ap exists at %s. Please remove or rename it before running.", afailedAPPath),
			})
		}
	}

	var warnFn func(string)
	if !e.silent {
		warnFn = e.warn
	}
	data, perr := Parse(e.patchContent, e.strict, warnFn)
	if perr != nil {
		return e.fatal(StatusFailed, "", false, 0, false, &AppError{Code: ErrInvalidPatchFile, Message: perr.Error()})
	}

	patchIDStr := data.PatchID
	if patchIDStr == "" {
		patchIDStr = "00000000"
	}

	for _, change := range data.Changes {
		if res := e.processChange(change); res != nil {
			return res
		}
	}

	if !e.strict && len(e.failedChangesOutput) > 0 {
		_ = writeAfailedAP(afailedAPPath, patchIDStr, e.failedChangesOutput)
		_ = writeLLMReport(e.afailedMDPath, e.patchContent, e.llmFileReports, nil, false)
		e.printf("\nWARNING: Some changes failed and were written to %s\n", afailedAPPath)
		e.printf("         A briefing for the generating model is in %s\n", e.afailedMDPath)
	}

	if e.dryRun {
		e.reportDryRun()
	} else if res := e.commit(); res != nil {
		return res
	}

	if !e.dryRun && len(e.failedChangesOutput) == 0 && pathExists(e.afailedMDPath) {
		_ = os.Remove(e.afailedMDPath)
	}

	if len(e.failedChangesOutput) > 0 {
		var failedFiles []string
		for _, b := range e.failedChangesOutput {
			failedFiles = append(failedFiles, b.FilePath)
		}
		return &Result{Status: StatusPartial, FailedFiles: failedFiles}
	}
	return &Result{Status: StatusSuccess}
}

// --- engine: per-Apply-call mutable state ----------------------------

type engine struct {
	strict     bool
	silent     bool
	dryRun     bool
	out        io.Writer
	projectDir string

	patchContent  string
	afailedMDPath string

	llmFileReports      []fileReport
	failedChangesOutput []*failedFileBlock
	writePlan           []writeOp
}

func (e *engine) printf(format string, args ...any) {
	if !e.silent {
		fmt.Fprintf(e.out, format, args...)
	}
}

func (e *engine) warn(msg string) {
	if !e.silent {
		fmt.Fprintf(e.out, "  [TOLERANT] %s\n", msg)
	}
}

func (e *engine) reportIdempotencySkip(reason string) {
	e.printf("  ~ SKIPPED (Idempotency): Looks like it's already applied. Reason: %s\n", reason)
}

func (e *engine) fatal(status Status, filePath string, hasFilePath bool, modIdx int, hasModIdx bool, aerr *AppError) *Result {
	fatal := &fatalInfo{HasFilePath: hasFilePath, FilePath: filePath, Err: aerr}
	_ = writeLLMReport(e.afailedMDPath, e.patchContent, e.llmFileReports, fatal, e.strict)
	e.printf("\nERROR: %s\n", aerr.Message)
	return &Result{Status: status, Error: aerr, FilePath: filePath, HasFilePath: hasFilePath, ModIdx: modIdx, HasModIdx: hasModIdx}
}

func (e *engine) addFailedWholeChange(relativePath, newline string, mods []*Modification) {
	e.failedChangesOutput = append(e.failedChangesOutput, &failedFileBlock{
		FilePath: relativePath, Newline: newline, Modifications: mods,
	})
}

func (e *engine) findOrCreateFailedBlock(relativePath, newline string) *failedFileBlock {
	for _, b := range e.failedChangesOutput {
		if b.FilePath == relativePath {
			return b
		}
	}
	b := &failedFileBlock{FilePath: relativePath, Newline: newline}
	e.failedChangesOutput = append(e.failedChangesOutput, b)
	return b
}

// --- write plan --------------------------------------------------------

type writeOpKind int

const (
	opDeletePath writeOpKind = iota
	opRename
	opCreateDir
	opWrite
)

type writeOp struct {
	kind    writeOpKind
	path    string
	newPath string
	content string
	relPath string
}

func (e *engine) commit() *Result {
	var deletes, renames, dirs, writes []writeOp
	for _, op := range e.writePlan {
		switch op.kind {
		case opDeletePath:
			deletes = append(deletes, op)
		case opRename:
			renames = append(renames, op)
		case opCreateDir:
			dirs = append(dirs, op)
		case opWrite:
			writes = append(writes, op)
		}
	}
	for _, op := range deletes {
		fi, err := os.Stat(op.path)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			err = os.RemoveAll(op.path)
		} else {
			err = os.Remove(op.path)
		}
		if err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileDeleteError, Message: err.Error()})
		}
	}
	for _, op := range renames {
		if !pathExists(op.path) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(op.newPath), 0o755); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileRenameError, Message: err.Error()})
		}
		if err := os.Rename(op.path, op.newPath); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileRenameError, Message: err.Error()})
		}
	}
	for _, op := range dirs {
		if err := os.MkdirAll(op.path, 0o755); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrDirCreateError, Message: err.Error()})
		}
	}
	for _, op := range writes {
		if err := os.MkdirAll(filepath.Dir(op.path), 0o755); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileWriteError, Message: err.Error()})
		}
		if err := os.WriteFile(op.path, []byte(op.content), 0o644); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileWriteError, Message: err.Error()})
		}
	}
	return nil
}

// reportDryRun prints one summary line per planned change instead of
// applying it, the DryRun counterpart of commit(). It mirrors the
// reference's "--- DRY RUN: planned changes ---" block (kind and path per
// op), but without the reference's per-file unified diff: the caller
// already has the confirmation dialog's own file list, and a paragraph of
// diff text does not fit a modal message box any better than this does.
func (e *engine) reportDryRun() {
	if len(e.writePlan) == 0 {
		return
	}
	e.printf("\n--- DRY RUN: planned changes ---\n")
	for _, op := range e.writePlan {
		switch op.kind {
		case opDeletePath:
			e.printf("delete %s\n", op.relPath)
		case opRename:
			newRel, err := filepath.Rel(e.projectDir, op.newPath)
			if err != nil {
				newRel = op.newPath
			}
			e.printf("rename %s -> %s\n", op.relPath, newRel)
		case opCreateDir:
			e.printf("create dir %s\n", op.relPath)
		case opWrite:
			e.printf("write %s\n", op.relPath)
		}
	}
}

// --- per-FILE-block dispatch --------------------------------------------

func isBareDelete(m *Modification) bool {
	return m.Snippet == nil && m.Anchor == nil && m.Content == nil && m.SnippetTail == nil &&
		m.IncludeLeadingBlankLines == 0 && m.IncludeTrailingBlankLines == 0 && m.ScopeEnd == 0
}

// processChange handles one FILE block. A non-nil return means a
// strict-mode fatal error occurred and Apply must return immediately
// (nothing gets written, per §3.0 atomicity).
func (e *engine) processChange(change *FileChange) *Result {
	if !change.HasFilePath {
		return e.fatal(StatusFailed, "", false, 0, false, &AppError{
			Code: ErrInvalidPatchFile, Message: "Missing 'file_path' for a change block.",
		})
	}
	originalRelativePath := change.FilePath
	isExplicitDir := strings.HasSuffix(originalRelativePath, "/") || strings.HasSuffix(originalRelativePath, "\\")
	relativePath := strings.TrimRight(originalRelativePath, "/\\")
	if relativePath == "" {
		relativePath = originalRelativePath
	}

	relativePath, strippedPrefix := resolvePathPrefix(e.projectDir, relativePath)

	filePath, secErr := securePath(e.projectDir, relativePath)
	if secErr != nil {
		aerr := &AppError{Code: ErrInvalidFilePath, Message: "Path traversal detected or invalid path format."}
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Path traversal detected or invalid path format.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	var newlineChar string
	switch change.Newline {
	case "LF":
		newlineChar = "\n"
	case "CRLF":
		newlineChar = "\r\n"
	case "CR":
		newlineChar = "\r"
	default:
		if pathExists(filePath) {
			newlineChar = detectLineEndings(filePath)
		} else {
			newlineChar = osLineSep()
		}
	}

	e.printf("\nFile: %s\n", relativePath)

	mods := change.Modifications
	// CONTEXTUAL FILE DELETION: a FILE block containing only a bare
	// DELETE (no other directives) deletes the whole file/directory.
	if len(mods) == 1 && mods[0].Action == "DELETE" && isBareDelete(mods[0]) {
		if !pathExists(filePath) {
			e.reportIdempotencySkip(fmt.Sprintf("Path to delete does not exist: %s", filePath))
			return nil
		}
		e.writePlan = append(e.writePlan, writeOp{kind: opDeletePath, path: filePath, relPath: relativePath})
		e.printf("  + SUCCESS: File deleted.\n")
		return nil
	}

	if change.RenameTo != nil {
		return e.processRename(change, relativePath, filePath, strippedPrefix)
	}

	return e.processModifications(change, relativePath, filePath, newlineChar, isExplicitDir)
}

func (e *engine) processRename(change *FileChange, relativePath, filePath, strippedPrefix string) *Result {
	newRelativePath := *change.RenameTo
	if strippedPrefix != "" {
		newParts := strings.Split(strings.ReplaceAll(newRelativePath, "\\", "/"), "/")
		prefixParts := strings.Split(strippedPrefix, "/")
		if len(newParts) >= len(prefixParts) && stringSlicesEqual(newParts[:len(prefixParts)], prefixParts) {
			newRelativePath = strings.Join(newParts[len(prefixParts):], "/")
		}
	}

	newFilePath, secErr := securePath(e.projectDir, newRelativePath)
	if secErr != nil {
		aerr := &AppError{Code: ErrInvalidFilePath, Message: "Path traversal detected in new rename path."}
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Path traversal detected in new rename path.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	if pathExists(newFilePath) {
		if !pathExists(filePath) {
			e.reportIdempotencySkip(fmt.Sprintf("Source does not exist, but destination does. Assuming rename complete: %s", newFilePath))
			return nil
		}
		aerr := &AppError{Code: ErrDestinationExists, Message: "Rename destination already exists."}
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Rename destination already exists.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	if !pathExists(filePath) {
		aerr := &AppError{Code: ErrFileNotFound, Message: "Target for rename not found."}
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Target for rename not found.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	e.writePlan = append(e.writePlan, writeOp{kind: opRename, path: filePath, newPath: newFilePath, relPath: relativePath})
	e.printf("  + SUCCESS: Renamed to %s\n", newRelativePath)
	return nil
}

// --- modification loop --------------------------------------------------

type modFailure struct {
	Idx int
	Mod *Modification
	Err *AppError
}

type consumedEntry struct {
	Idx     int
	Removed string
}

type locatorTriple struct{ anchor, snippet, tail string }

func locatorKey(m *Modification) locatorTriple {
	return locatorTriple{
		normalizeBlock(derefOr(m.Anchor, "")),
		normalizeBlock(derefOr(m.Snippet, "")),
		normalizeBlock(derefOr(m.SnippetTail, "")),
	}
}

func actionOrUnknown(a string) string {
	if a == "" {
		return "Unknown"
	}
	return a
}

func (e *engine) processModifications(change *FileChange, relativePath, filePath, newlineChar string, isExplicitDir bool) *Result {
	fileExisted := pathExists(filePath)
	var originalContent string
	if fileExisted && isDirPath(filePath) {
		// A directory at the target path: leave originalContent empty; the
		// logic below (CREATE/RECREATE on it) will surface the right errors.
	} else if fileExisted {
		b, rerr := os.ReadFile(filePath)
		if rerr == nil {
			originalContent = normalizeToLF(string(b))
		}
	} else {
		hasCreateOrRecreate := false
		for _, m := range change.Modifications {
			if m.Action == "CREATE" || m.Action == "RECREATE" {
				hasCreateOrRecreate = true
				break
			}
		}
		if !hasCreateOrRecreate {
			aerr := &AppError{Code: ErrFileNotFound, Message: "Target file not found."}
			if e.strict {
				return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
			}
			e.printf("  - FAILED: Target file not found.\n")
			e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
			return nil
		}
	}

	workingContent := originalContent
	initialContent := workingContent
	var consumedLog []consumedEntry

	seen := map[locatorTriple]bool{}
	repeated := map[locatorTriple]bool{}
	for _, m := range change.Modifications {
		if derefOr(m.Snippet, "") == "" && derefOr(m.Anchor, "") == "" {
			continue
		}
		k := locatorKey(m)
		if seen[k] {
			repeated[k] = true
		}
		seen[k] = true
	}

	var dirtyRegions []span
	terminalOpPlanned := false

	pendingIdx := make([]int, len(change.Modifications))
	for i := range pendingIdx {
		pendingIdx[i] = i
	}
	var finalFailedMods []modFailure
	passNumber := 1

	for len(pendingIdx) > 0 {
		madeProgress := false
		var failedInThisPass []modFailure
		lastModEndPos := 0

		for _, modIdx := range pendingIdx {
			mod := change.Modifications[modIdx]
			res, failure, progressed, newLastEnd, stop := e.applyOneModification(
				change, mod, modIdx, relativePath, filePath, isExplicitDir, fileExisted,
				originalContent, &workingContent, initialContent, &dirtyRegions, &consumedLog,
				repeated[locatorKey(mod)], lastModEndPos, passNumber, &terminalOpPlanned)
			if stop {
				return res
			}
			if failure != nil {
				failedInThisPass = append(failedInThisPass, *failure)
				continue
			}
			if progressed {
				madeProgress = true
			}
			lastModEndPos = newLastEnd
		}

		if !madeProgress {
			finalFailedMods = failedInThisPass
			break
		}
		pendingIdx = make([]int, len(failedInThisPass))
		for i, f := range failedInThisPass {
			pendingIdx[i] = f.Idx
		}
		passNumber++
	}

	if len(finalFailedMods) > 0 {
		var failedItems []failedItem
		for _, f := range finalFailedMods {
			failedItems = append(failedItems, failedItem{ModIdx: f.Idx, Mod: f.Mod, Err: f.Err})
		}
		e.llmFileReports = append(e.llmFileReports, fileReport{
			FilePath: relativePath, Original: originalContent, Current: workingContent,
			TotalMods: len(change.Modifications), Failed: failedItems,
		})
		for _, f := range finalFailedMods {
			e.printf("  - FAILED: Mod #%d (%s). Reason: %s\n", f.Idx+1, actionOrUnknown(f.Mod.Action), f.Err.Message)
			block := e.findOrCreateFailedBlock(relativePath, change.Newline)
			block.Modifications = append(block.Modifications, f.Mod)
		}
	}

	// STRUCTURAL SANITY CHECK: a file balanced before the patch and not
	// balanced after it is almost certainly no longer valid source, even
	// though every modification applied cleanly.
	if fileExisted && workingContent != initialContent && !e.silent {
		before := netBracketDepth(initialContent)
		after := netBracketDepth(workingContent)
		if before == 0 && after != 0 {
			e.printf("  ! WARNING: brackets in %s were balanced before the patch and are off by %+d "+
				"after it. The result is very likely not valid source code - review it before committing.\n",
				relativePath, after)
		}
	}

	if !terminalOpPlanned {
		finalContent := strings.Join(strings.Split(workingContent, "\n"), newlineChar)
		if finalContent != originalContent || !fileExisted {
			if finalContent != "" && !strings.HasSuffix(finalContent, newlineChar) {
				finalContent += newlineChar
			}
			e.writePlan = append(e.writePlan, writeOp{kind: opWrite, path: filePath, content: finalContent, relPath: relativePath})
		}
	}

	return nil
}
