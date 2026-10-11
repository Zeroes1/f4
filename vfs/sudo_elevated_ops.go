package vfs

import (
	"os"
	"path/filepath"
	"runtime"
)

// ElevatedFileOps answers the file operations of the elevated dispatcher
// (f4#1768, step 3): Stat, ReadDir, MkDir, Remove, Rename, SetAttributes,
// Symlink and Hardlink, done with the dispatcher's rights. It is the same work
// the Unix dispatcher does in handleSudoClient, written once for every
// platform. A file's contents cannot travel as a descriptor between processes
// on Windows (CmdOpen and CmdCreate pass one with SCM_RIGHTS on Unix), so they
// are refused here until reading and writing by requests is added.
func ElevatedFileOps(req SudoRequest) SudoResponse {
	var resp SudoResponse
	fail := func(err error) {
		if err != nil {
			resp.Error = err.Error()
		}
	}
	switch req.Cmd {
	case CmdPing:
	case CmdStat:
		info, err := os.Stat(req.Path)
		if err != nil {
			fail(err)
			break
		}
		resp.Item = elevatedItem(req.Path, info)
	case CmdReadDir:
		entries, err := os.ReadDir(req.Path)
		if err != nil {
			fail(err)
			break
		}
		for _, e := range entries {
			full := filepath.Join(req.Path, e.Name())
			info, err := e.Info()
			if err != nil {
				resp.Items = append(resp.Items, VFSItem{Name: e.Name(), IsDir: e.IsDir(), KnownMetadata: MetadataExplicit})
				continue
			}
			item := elevatedItem(full, info)
			item.IsSymlink = e.Type()&os.ModeSymlink != 0
			// A link or another special object is a directory when what it
			// points at is one.
			if !item.IsDir && !e.Type().IsRegular() {
				if target, err := os.Stat(full); err == nil {
					item.IsDir = target.IsDir()
				}
			}
			resp.Items = append(resp.Items, item)
		}
	case CmdMkDir:
		fail(os.MkdirAll(req.Path, os.FileMode(req.Mode)))
	case CmdRemove:
		fail(os.RemoveAll(req.Path))
	case CmdRename:
		if req.Flags&SudoRenameNoReplace != 0 {
			fail(renameNoReplace(req.Path, req.Path2))
		} else {
			fail(os.Rename(req.Path, req.Path2))
		}
	case CmdSymlink:
		fail(os.Symlink(req.Path2, req.Path))
	case CmdHardlink:
		fail(os.Link(req.Path2, req.Path))
	case CmdSetAttributes:
		err := os.Chmod(req.Path, os.FileMode(req.Item.UnixMode))
		if err == nil && runtime.GOOS != "windows" {
			err = os.Chown(req.Path, req.Item.Uid, req.Item.Gid)
		}
		if err == nil {
			err = os.Chtimes(req.Path, req.Item.ATime, req.Item.MTime)
		}
		if err == nil {
			err = applyPlatformAttributes(req.Path, req.Item)
		}
		fail(err)
	default:
		resp.Error = "operation not supported by the elevated dispatcher yet"
	}
	return resp
}

// elevatedItem describes path the way the panel lists it.
func elevatedItem(path string, info os.FileInfo) VFSItem {
	item := VFSItem{
		KnownMetadata: MetadataExplicit | MetadataPermissions | MetadataMTime | MetadataHidden | MetadataExecutable,
		SizeKnown:     true,
		UnixMode:      uint32(info.Mode().Perm()),
		Name:          info.Name(),
		Size:          info.Size(),
		IsDir:         info.IsDir(),
		MTime:         info.ModTime(),
		IsExecutable:  info.Mode().Perm()&0o111 != 0,
		IsHidden:      isHidden(path, info.Name(), info),
	}
	fillPlatformTimes(&item, info)
	fillPhysicalSizeCheap(&item, info)
	return item
}
