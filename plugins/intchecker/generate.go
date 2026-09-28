package intchecker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// outputMode is where "Generate hashes" puts the checksums. The values double
// as radio button indexes in the dialog, in IntChecker's order.
type outputMode int

const (
	// outputSingle stores every checksum in one file named in the dialog.
	outputSingle outputMode = iota
	// outputSeparate stores "<file><ext>" next to every hashed file.
	outputSeparate
	// outputDirectory stores one "<directory name><ext>" in every directory
	// that holds hashed files.
	outputDirectory
	// outputDisplay writes nothing and shows the list in a window.
	outputDisplay
)

func (m outputMode) valid() bool { return m >= outputSingle && m <= outputDisplay }

// writesManyFiles says whether the mode creates one checksum file per file or
// per directory (as opposed to the one named file, or none at all).
func (m outputMode) writesManyFiles() bool { return m == outputSeparate || m == outputDirectory }

// generateJob describes one "Generate hashes" run: hash names (relative to dir
// on fs) and store the result as mode says. output is the file name of the
// "Single file" mode, in the same directory.
type generateJob struct {
	fs        vfs.VFS
	dir       string
	names     []string
	algorithm Algorithm
	mode      outputMode
	output    string
	// overwrite says the user agreed to replace existing checksum files.
	// Without it a checksum file is created only if it does not exist, so a
	// file that appeared while hashing is never clobbered.
	overwrite bool
	// skipExisting says the user chose to keep the checksum files that
	// already exist (one-file-per-file and per-directory modes): those are
	// left alone and reported as skipped.
	skipExisting bool
}

// fileFailure is a file that could not be hashed or written, and why.
type fileFailure struct {
	Name string
	Err  error
}

// generateResult is what a run did.
type generateResult struct {
	Written  int           // entries stored in checksum files (or shown)
	Skipped  []string      // directories left out (no recursion yet)
	Failures []fileFailure // files that could not be read
	// Outputs are the checksum files written, relative to the job's dir.
	Outputs []string
	// SkippedExisting are checksum files kept because they already existed
	// and the user chose to skip them.
	SkippedExisting []string
	// WriteFailures are checksum files that could not be written; the
	// others are still written.
	WriteFailures []fileFailure
	// Text is the checksum list of the "Display" mode.
	Text string
}

// errNothingToHash means no regular file was left to hash.
var errNothingToHash = errors.New("no files to hash")

// progressReporter is the part of vfs.TaskReporter a run uses.
type progressReporter interface {
	UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string)
}

const hashBufferSize = 1 << 20

// outputFor says which checksum file stores the entry name (relative to the
// job's dir, '/'-separated) and under which name the entry is stored there.
// The display mode has no file.
func (job generateJob) outputFor(name string) (target, stored string) {
	switch job.mode {
	case outputSingle:
		return job.output, name
	case outputSeparate:
		return name + job.algorithm.Extension(), path.Base(name)
	case outputDirectory:
		sub := path.Dir(name)
		if sub == "." {
			return defaultOutputName(job.fs.Base(job.dir), job.algorithm), name
		}
		return path.Join(sub, defaultOutputName(path.Base(sub), job.algorithm)), path.Base(name)
	}
	return "", name
}

// targetPath is the full path of a checksum file named relative to job.dir.
func (job generateJob) targetPath(target string) string {
	return job.fs.Join(append([]string{job.dir}, strings.Split(target, "/")...)...)
}

// hashInput is a regular file a run hashes.
type hashInput struct {
	name string
	size int64
}

// collectInputs stats the job's names and returns the regular files to hash,
// sorted by name. Directories go to res.Skipped, names that cannot be stat'ed
// to res.Failures, and a file that is itself one of the run's checksum files
// (a list selected together with the files it lists) is left out.
func collectInputs(ctx context.Context, job generateJob, res *generateResult) ([]hashInput, error) {
	var inputs []hashInput
	for _, name := range job.names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if name == "" || name == ".." {
			continue
		}
		item, err := job.fs.Stat(ctx, job.fs.Join(job.dir, name))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			res.Failures = append(res.Failures, fileFailure{Name: name, Err: err})
			continue
		}
		if item.IsDir {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		inputs = append(inputs, hashInput{name: name, size: item.Size})
	}
	if job.mode != outputDisplay {
		targets := make(map[string]bool, len(inputs))
		for _, in := range inputs {
			target, _ := job.outputFor(in.name)
			targets[target] = true
		}
		kept := inputs[:0]
		for _, in := range inputs {
			if !targets[in.name] {
				kept = append(kept, in)
			}
		}
		inputs = kept
	}
	// The panel's marking order is incidental; a sorted file diffs cleanly
	// and reads the same whichever way the files were marked.
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].name < inputs[j].name })
	return inputs, nil
}

// existingOutputs lists the checksum files of the job that already exist, so
// the user can be asked before anything is hashed.
func existingOutputs(ctx context.Context, job generateJob) ([]string, error) {
	if job.mode == outputDisplay {
		return nil, nil
	}
	if job.mode == outputSingle {
		if _, err := job.fs.Stat(ctx, job.targetPath(job.output)); err == nil {
			return []string{job.output}, nil
		}
		return nil, ctx.Err()
	}
	var ignored generateResult
	inputs, err := collectInputs(ctx, job, &ignored)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var existing []string
	for _, in := range inputs {
		target, _ := job.outputFor(in.name)
		if seen[target] {
			continue
		}
		seen[target] = true
		if _, err := job.fs.Stat(ctx, job.targetPath(target)); err == nil {
			existing = append(existing, target)
		} else if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}
	return existing, nil
}

// runGenerate hashes the job's files and writes the checksum files (or, for
// the display mode, puts the list into res.Text). Checksum files are written
// only once everything is hashed, so a run cancelled while hashing (ctx done)
// returns ctx.Err() and writes nothing; one cancelled among the per-file or
// per-directory writes keeps the files already written (res.Outputs).
func runGenerate(ctx context.Context, job generateJob, reporter progressReporter) (generateResult, error) {
	var res generateResult
	if !job.algorithm.valid() {
		return res, fmt.Errorf("unknown checksum algorithm %d", int(job.algorithm))
	}
	if !job.mode.valid() {
		return res, fmt.Errorf("unknown output mode %d", int(job.mode))
	}
	inputs, err := collectInputs(ctx, job, &res)
	if err != nil {
		return res, err
	}
	if len(inputs) == 0 {
		if len(res.Failures) > 0 {
			return res, nil
		}
		return res, errNothingToHash
	}
	var total int64
	for _, in := range inputs {
		total += in.size
	}

	action := vtui.Msg("IntChecker.ProgressHashing")
	var done int64
	buf := make([]byte, hashBufferSize)
	entries := make([]Entry, 0, len(inputs))
	for i, in := range inputs {
		report := func(fileDone int64) {
			totalText := fmt.Sprintf(vtui.Msg("IntChecker.ProgressFiles"), i+1, len(inputs))
			reporter.UpdateTransfer(action, in.name, percent(fileDone, in.size), totalText, percent(done+fileDone, total), "")
		}
		report(0)
		sum, read, err := hashFile(ctx, job.fs, job.fs.Join(job.dir, in.name), job.algorithm, buf, report)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return res, ctxErr
			}
			res.Failures = append(res.Failures, fileFailure{Name: in.name, Err: err})
			done += in.size
			continue
		}
		done += read
		entries = append(entries, Entry{Name: in.name, Sum: sum})
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if len(entries) == 0 {
		return res, nil
	}

	switch job.mode {
	case outputDisplay:
		data, err := FormatHashFile(job.algorithm, entries)
		if err != nil {
			return res, err
		}
		res.Text = string(data)
		res.Written = len(entries)
		return res, nil
	case outputSingle:
		data, err := FormatHashFile(job.algorithm, entries)
		if err != nil {
			return res, err
		}
		if err := writeFile(ctx, job.fs, job.targetPath(job.output), data, job.overwrite); err != nil {
			return res, err
		}
		res.Written = len(entries)
		res.Outputs = []string{job.output}
		return res, nil
	}
	return res, writeManyOutputs(ctx, job, entries, &res)
}

// writeManyOutputs groups entries by checksum file and writes every file. One
// file that cannot be written does not stop the others; it is reported in
// res.WriteFailures.
func writeManyOutputs(ctx context.Context, job generateJob, entries []Entry, res *generateResult) error {
	groups := map[string][]Entry{}
	var order []string
	for _, e := range entries {
		target, stored := job.outputFor(e.Name)
		if _, ok := groups[target]; !ok {
			order = append(order, target)
		}
		groups[target] = append(groups[target], Entry{Name: stored, Sum: e.Sum})
	}
	sort.Strings(order)
	for _, target := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		full := job.targetPath(target)
		if job.skipExisting {
			if _, err := job.fs.Stat(ctx, full); err == nil {
				res.SkippedExisting = append(res.SkippedExisting, target)
				continue
			}
		}
		data, err := FormatHashFile(job.algorithm, groups[target])
		if err == nil {
			err = writeFile(ctx, job.fs, full, data, job.overwrite)
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			res.WriteFailures = append(res.WriteFailures, fileFailure{Name: target, Err: err})
			continue
		}
		res.Written += len(groups[target])
		res.Outputs = append(res.Outputs, target)
	}
	return nil
}

// hashFile reads path to the end through fs and returns its digest and the
// number of bytes read. progress is called with the bytes read so far.
func hashFile(ctx context.Context, fs vfs.VFS, path string, a Algorithm, buf []byte, progress func(int64)) (sum []byte, read int64, err error) {
	f, err := fs.Open(ctx, path)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	h := a.New()
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, read, ctxErr
		}
		n, readErr := f.Read(ctx, buf)
		if n > 0 {
			_, _ = h.Write(buf[:n]) // hash.Hash.Write never fails
			read += int64(n)
			progress(read)
		}
		if readErr == io.EOF {
			return h.Sum(nil), read, nil
		}
		if readErr != nil {
			return nil, read, readErr
		}
	}
}

// writeFile stores data at path. Without overwrite an existing file is left
// untouched and the call fails.
func writeFile(ctx context.Context, fs vfs.VFS, path string, data []byte, overwrite bool) (err error) {
	w, err := fs.Create(vfs.WithDestinationOverwrite(ctx, overwrite), path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := w.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	_, err = w.Write(data)
	return err
}

// percent is done/total as 0..100; an empty total counts as complete.
func percent(done, total int64) int {
	if total <= 0 {
		return 100
	}
	if done >= total {
		return 100
	}
	return int(done * 100 / total)
}
