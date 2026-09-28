package intchecker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// generateJob describes one "Generate hashes" run with the "Single file"
// output: hash names (relative to dir on fs) and store the result in output,
// a file name in the same directory.
type generateJob struct {
	fs        vfs.VFS
	dir       string
	names     []string
	algorithm Algorithm
	output    string
	// overwrite says the user agreed to replace an existing output file.
	// Without it the output is created only if it does not exist, so a file
	// that appeared while hashing is never clobbered.
	overwrite bool
}

// fileFailure is a file that could not be hashed, and why.
type fileFailure struct {
	Name string
	Err  error
}

// generateResult is what a run did.
type generateResult struct {
	Written  int           // entries stored in the checksum file
	Skipped  []string      // directories left out (no recursion yet)
	Failures []fileFailure // files that could not be read
}

// errNothingToHash means no regular file was left to hash.
var errNothingToHash = errors.New("no files to hash")

// progressReporter is the part of vfs.TaskReporter a run uses.
type progressReporter interface {
	UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string)
}

const hashBufferSize = 1 << 20

// runGenerate hashes the job's files and writes the checksum file. The file
// is written only once everything is hashed, so a cancelled run (ctx done)
// returns ctx.Err() and writes nothing.
func runGenerate(ctx context.Context, job generateJob, reporter progressReporter) (generateResult, error) {
	var res generateResult
	if !job.algorithm.valid() {
		return res, fmt.Errorf("unknown checksum algorithm %d", int(job.algorithm))
	}

	type input struct {
		name string
		size int64
	}
	var inputs []input
	var total int64
	for _, name := range job.names {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if name == "" || name == ".." || name == job.output {
			continue
		}
		item, err := job.fs.Stat(ctx, job.fs.Join(job.dir, name))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return res, ctxErr
			}
			res.Failures = append(res.Failures, fileFailure{Name: name, Err: err})
			continue
		}
		if item.IsDir {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		inputs = append(inputs, input{name: name, size: item.Size})
		total += item.Size
	}
	if len(inputs) == 0 {
		if len(res.Failures) > 0 {
			return res, nil
		}
		return res, errNothingToHash
	}
	// The panel's marking order is incidental; a sorted file diffs cleanly
	// and reads the same whichever way the files were marked.
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].name < inputs[j].name })

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

	data, err := FormatHashFile(job.algorithm, entries)
	if err != nil {
		return res, err
	}
	if err := writeFile(ctx, job.fs, job.fs.Join(job.dir, job.output), data, job.overwrite); err != nil {
		return res, err
	}
	res.Written = len(entries)
	return res, nil
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
