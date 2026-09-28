package panel

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// treeChildScanCap bounds how many directory entries a single expand reads
// before giving up on that one directory -- a defensive limit against a
// pathological single directory (e.g. a maildir with tens of thousands of
// entries), not a feature. It does not bound the tree as a whole: expansion
// is lazy (treeItem.collapsed), so the overall tree size is however much
// the user has actually expanded, never the whole disk at once.
const treeChildScanCap = 20000

// treeItem is one row of the flat tree list. Modeled directly on far2l's
// own tree panel (far2l/src/panels/treelist.cpp, treelist.hpp -- see
// f4#1602's tracking comment): the tree is a flat, index-addressed list,
// not a nested structure, so expanding or collapsing a node only ever
// splices a contiguous run of its descendants in or out of this same list.
// Nothing here ever walks the whole subtree on a keypress or a render --
// only expanding a node reads that one directory's immediate children.
type treeItem struct {
	name string
	path string
	// depth is the item's nesting level; the root is 0.
	depth int
	// parentIndex is the row index of this item's parent in the flat list,
	// -1 for the root. Used to find a node's contiguous descendant run
	// (collapseAt) and to fix up indices after a splice (spliceChildren).
	parentIndex int
	// expandable is true while this directory might still have
	// subdirectories. It starts optimistic (every directory is assumed
	// expandable) and is corrected to false the first time expandAt
	// actually reads an empty result -- avoiding a second stat/readdir just
	// to answer "does it have children" up front.
	expandable bool
	// collapsed is true while expandable's children are not currently
	// spliced into the list. Meaningless when expandable is false.
	collapsed bool
	// last records, for each ancestor level from the root down to (but not
	// including) this item, whether that ancestor was the last child among
	// its own siblings -- len(last) == depth. last[depth-1] (the item's own
	// slot) says whether THIS item is the last child, deciding its own
	// connector glyph (└─ vs ├─); last[0..depth-2] says whether each
	// ancestor's vertical guide line continues past this row or stops
	// (blank), the standard "tree" / far2l style of connector rendering.
	last []bool
}

// GetCellText implements vtui.TableRow, rendering the ASCII tree connectors
// from depth+last, an expand-state marker, and the name -- see treeItem's
// own doc comment for what each piece of state means.
func (it treeItem) GetCellText(int) string {
	var b strings.Builder
	for i := 0; i < it.depth-1; i++ {
		if it.last[i] {
			b.WriteString("  ")
		} else {
			b.WriteString("\u2502 ") // │
		}
	}
	if it.depth > 0 {
		if it.last[it.depth-1] {
			b.WriteString("\u2514\u2500") // └─
		} else {
			b.WriteString("\u251c\u2500") // ├─
		}
	}
	switch {
	case it.expandable && it.collapsed:
		b.WriteString("+ ")
	case it.expandable && !it.collapsed:
		b.WriteString("- ")
	default:
		b.WriteString("  ")
	}
	b.WriteString(it.name)
	return b.String()
}

// TreePanel is far2l/DOS Navigator's directory tree (far2l: Alt+F10; DOS
// Navigator has an equivalent tree view), bound to Ctrl+T -- see AltPanel's
// own doc comment, which already named "tree" alongside "info"/"quick_view"
// as a planned Kind. First atomic part of the DOS Navigator ideas ticket
// (f4#1602). The tree is rooted at the source panel's current volume (its
// filesystem root on POSIX, the drive root on Windows) -- matching far2l's
// own "tree of the current disk" scope -- and lazily expanded: only the
// root and the chain of directories down to the source panel's current
// directory are read up front (revealPath), everything else is read the
// first time the user expands it (Right arrow / expandAt).
//
// Enter on a row changes Source()'s directory to that row's path and closes
// the tree, the same "pick a target, then get out of the way" round trip
// bookmarks_dialog.go's Enter handling already does via NavigateToBookmark.
// Right/Left expand/collapse the row under the cursor without navigating.
//
// Scope for this part: a persistent expand/collapse cache across tree
// panel instances and a per-plugin f4:config knob (e.g. root = current dir
// instead of the whole volume) are follow-up parts (see f4#1602).
type TreePanel struct {
	src     *FileSystemPanel
	Frame   *vtui.BorderedFrame
	Table   *vtui.Table
	Focused bool

	root  string
	items []treeItem
}

// NewTreePanel builds the tree for src's current volume, pre-expanded down
// to src's current directory. Called by PanelsFrame.ToggleAltPanel
// (Ctrl+T), which also takes care of placement, sizing and redraw --
// SetPosition below only needs to be internally consistent, not final.
func NewTreePanel(src *FileSystemPanel) *TreePanel {
	x1, y1, x2, y2 := src.GetPosition()
	t := &TreePanel{src: src, root: treeVolumeRoot(treeRootFor(src))}

	t.Frame = vtui.NewBorderedFrame(x1, y1, x2, y2, vtui.SingleBox, i18n.Msg("TreePanel.Title"))
	t.Frame.ColorBoxIdx = theme.ColPanelBox
	t.Frame.ColorTitleIdx = theme.ColPanelTitle

	t.Table = vtui.NewTable(0, 0, 1, 1, []vtui.TableColumn{{Title: i18n.Msg("TreePanel.ColumnName")}})
	t.Table.ShowHeader = false
	t.Table.QuickSearch = true
	t.Table.ColorTextIdx = theme.ColPanelText
	t.Table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	t.Table.ColorBoxIdx = theme.ColPanelBox

	t.items = []treeItem{{name: t.root, path: t.root, depth: 0, parentIndex: -1, expandable: true, collapsed: true}}
	t.expandAt(0)
	t.revealPath(treeRootFor(src))
	t.syncRows()

	t.SetFocus(false)
	t.SetPosition(x1, y1, x2, y2)
	return t
}

func treeRootFor(src *FileSystemPanel) string {
	if src == nil || src.Vfs == nil {
		return ""
	}
	return src.Vfs.GetPath()
}

// treeVolumeRoot returns path's volume root: "/" on POSIX (VolumeName is
// always "" there), the drive root ("C:\") on Windows.
func treeVolumeRoot(path string) string {
	if vol := filepath.VolumeName(path); vol != "" {
		return vol + string(filepath.Separator)
	}
	return string(filepath.Separator)
}

// scanChildDirs lists dir's immediate subdirectories, sorted
// case-insensitively. A symlink to a directory reports IsDir() == false on
// the fs.DirEntry os.ReadDir returns (its type bit is DT_LNK, not DT_DIR,
// on the platforms f4 targets), so this walk skips symlinked directories
// rather than following them into a possible cycle, with no separate check
// needed.
func scanChildDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
		if len(names) >= treeChildScanCap {
			break
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}

// expandAt reads items[at]'s immediate subdirectories (if any) and splices
// them into the flat list right after it, correcting parentIndex on every
// item that used to sit after the splice point. A no-op if the item is
// already expanded or not expandable.
func (t *TreePanel) expandAt(at int) {
	if at < 0 || at >= len(t.items) {
		return
	}
	item := &t.items[at]
	if !item.expandable || !item.collapsed {
		return
	}
	names := scanChildDirs(item.path)
	if len(names) == 0 {
		item.expandable = false
		return
	}

	children := make([]treeItem, len(names))
	for i, name := range names {
		last := append(append([]bool{}, item.last...), i == len(names)-1)
		children[i] = treeItem{
			name:        name,
			path:        filepath.Join(item.path, name),
			depth:       item.depth + 1,
			parentIndex: at,
			expandable:  true,
			collapsed:   true,
			last:        last,
		}
	}
	t.spliceChildren(at, children)
	t.items[at].collapsed = false
}

// spliceChildren inserts children right after items[at], shifting
// parentIndex on every existing item that pointed past at (i.e. whose
// parent -- or an ancestor's position -- moved).
func (t *TreePanel) spliceChildren(at int, children []treeItem) {
	n := len(children)
	for i := range t.items {
		if t.items[i].parentIndex > at {
			t.items[i].parentIndex += n
		}
	}
	tail := append([]treeItem{}, t.items[at+1:]...)
	merged := append([]treeItem{}, t.items[:at+1]...)
	merged = append(merged, children...)
	merged = append(merged, tail...)
	t.items = merged
}

// collapseAt removes items[at]'s descendant run (the contiguous items right
// after it whose depth is greater) from the flat list, fixing up
// parentIndex on everything that pointed past the removed range. A no-op if
// the item has no children currently spliced in.
func (t *TreePanel) collapseAt(at int) {
	if at < 0 || at >= len(t.items) || t.items[at].collapsed {
		return
	}
	depth := t.items[at].depth
	end := at + 1
	for end < len(t.items) && t.items[end].depth > depth {
		end++
	}
	removed := end - (at + 1)
	if removed > 0 {
		t.items = append(t.items[:at+1], t.items[end:]...)
		for i := range t.items {
			if t.items[i].parentIndex >= end {
				t.items[i].parentIndex -= removed
			}
		}
	}
	t.items[at].collapsed = true
}

// revealPath expands the chain of directories from the root down to
// target (normally src's current directory), one level at a time, and
// leaves the cursor on target's own row. It stops silently at whatever
// level it can no longer match or expand -- e.g. target outside root's
// volume, or a permission error on a directory the source panel itself
// somehow still reached -- rather than fail the whole tree open.
//
// Matching each chain component against the already-expanded children
// (findChild) is name-based, which two platform quirks can defeat even
// though target names a real, existing directory the source panel is
// already sitting in (see f4#1602's tracking comment for the CI failure
// this fixes): a path component that is itself a symlink to a directory
// (macOS routes every os.TempDir()-derived path through /var, itself a
// symlink to /private/var, and scanChildDirs deliberately never lists a
// symlinked directory -- see its own doc comment -- to avoid following one
// into a cycle during ordinary browsing), and a short (8.3-style) Windows
// path component (e.g. "RUNNER~1" for "runneradmin", which is what GitHub's
// Windows runners set %TEMP% to) that never appears literally in
// os.ReadDir's listing either. revealChild covers both by falling back to
// os.Stat, which -- unlike scanChildDirs -- follows a symlink and accepts a
// short name.
func (t *TreePanel) revealPath(target string) {
	root := strings.TrimRight(t.root, string(filepath.Separator))
	target = strings.TrimRight(filepath.Clean(target), string(filepath.Separator))
	rel := strings.TrimPrefix(target, root)
	rel = strings.Trim(rel, string(filepath.Separator))
	if rel == "" {
		return
	}
	parts := strings.Split(rel, string(filepath.Separator))
	current := 0
	for _, part := range parts {
		t.expandAt(current)
		next := t.findChild(current, part)
		if next == -1 {
			next = t.revealChild(current, part)
		}
		if next == -1 {
			return
		}
		current = next
	}
	t.setCursor(current)
}

// findChild returns the row index of items[parent]'s already-listed child
// named name, or -1 if there is none. Matching is case-insensitive on
// Windows, where the very listing being searched (scanChildDirs's) already
// came from a case-preserving but case-insensitive filesystem, so two
// spellings of the same name are the same child.
func (t *TreePanel) findChild(parent int, name string) int {
	for i := parent + 1; i < len(t.items) && t.items[i].parentIndex >= parent; i++ {
		if t.items[i].parentIndex == parent && pathNamesEqual(t.items[i].name, name) {
			return i
		}
	}
	return -1
}

// pathNamesEqual compares one path component of a revealPath chain against
// a candidate child name. Case-insensitive on Windows (see findChild);
// exact everywhere else, where case is part of a file's identity.
func pathNamesEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// revealChild splices in a single node for name under items[parent], for
// the case findChild alone cannot handle: name is a real subdirectory of
// items[parent].path that scanChildDirs's own listing does not surface
// (see revealPath's doc comment for why -- a symlinked directory on macOS,
// a short Windows name). os.Stat, unlike os.ReadDir's DirEntry, follows a
// symlink and accepts a short name, so it can confirm the directory exists
// without needing it to appear in that filtered listing -- and because this
// single bounded lookup is anchored to the one already-real target path
// revealPath is unwinding, not a general directory walk, following that
// one symlink here carries none of scanChildDirs's own cycle risk. Returns
// -1 if name is not actually a subdirectory there.
func (t *TreePanel) revealChild(parent int, name string) int {
	path := filepath.Join(t.items[parent].path, name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return -1
	}

	depth := t.items[parent].depth
	end := parent + 1
	for end < len(t.items) && t.items[end].depth > depth {
		end++
	}
	lastSibling := -1
	for i := parent + 1; i < end; i++ {
		if t.items[i].parentIndex == parent {
			lastSibling = i
		}
	}
	if lastSibling != -1 {
		t.items[lastSibling].last[t.items[lastSibling].depth-1] = false
	}

	child := treeItem{
		name:        name,
		path:        path,
		depth:       depth + 1,
		parentIndex: parent,
		expandable:  true,
		collapsed:   true,
		last:        append(append([]bool{}, t.items[parent].last...), true),
	}
	t.spliceChildren(end-1, []treeItem{child})
	t.items[parent].expandable = true
	t.items[parent].collapsed = false
	return end
}

func (t *TreePanel) setCursor(idx int) {
	if idx < 0 || idx >= len(t.items) {
		return
	}
	t.Table.SelectPos = idx
	t.Table.EnsureVisible()
}

func (t *TreePanel) syncRows() {
	rows := make([]vtui.TableRow, len(t.items))
	for i, it := range t.items {
		rows[i] = it
	}
	t.Table.SetRows(rows)
}

func (t *TreePanel) cursorIndex() int {
	idx := t.Table.RowAt(t.Table.SelectPos)
	if idx < 0 || idx >= len(t.items) {
		return -1
	}
	return idx
}

func (t *TreePanel) Source() *FileSystemPanel { return t.src }
func (t *TreePanel) Kind() string             { return "tree" }

func (t *TreePanel) SetPosition(x1, y1, x2, y2 int) {
	t.Frame.SetPosition(x1, y1, x2, y2)
	b := t.Frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	t.Table.SetPosition(ix1, iy1, ix2, iy2)
}

func (t *TreePanel) GetPosition() (int, int, int, int) { return t.Frame.GetPosition() }

// SetFocus flips the visible focus marker -- the frame title recolours the
// same way InfoPanel/QuickViewPanel's do, and the cursor row switches
// between the active/inactive cursor colors the same way
// FileSystemPanel.SetFocus (list.go) and ProcList's own panel do for their
// own vtui.Table.
func (t *TreePanel) SetFocus(f bool) {
	t.Focused = f
	if f {
		t.Frame.ColorTitleIdx = theme.ColPanelSelectedTitle
		t.Table.ColorSelectedTextIdx = theme.ColPanelCursor
		t.Table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	} else {
		t.Frame.ColorTitleIdx = theme.ColPanelTitle
		t.Table.ColorSelectedTextIdx = theme.ColPanelInactiveCursor
		t.Table.ColorItemSelectCursorIdx = theme.ColPanelInactiveSelectedCursor
	}
	t.Table.SetFocus(f)
}

func (t *TreePanel) IsFocused() bool { return t.Focused }

func (t *TreePanel) ProcessKey(e *vtinput.InputEvent) bool {
	if e != nil && e.KeyDown {
		switch e.VirtualKeyCode {
		case vtinput.VK_RETURN:
			t.activateSelected()
			return true
		case vtinput.VK_RIGHT:
			idx := t.cursorIndex()
			if idx >= 0 && t.items[idx].expandable && t.items[idx].collapsed {
				t.expandAt(idx)
				t.syncRows()
				t.setCursor(idx)
			}
			return true
		case vtinput.VK_LEFT:
			idx := t.cursorIndex()
			if idx >= 0 && !t.items[idx].collapsed {
				t.collapseAt(idx)
				t.syncRows()
				t.setCursor(idx)
			} else if idx > 0 {
				t.setCursor(t.items[idx].parentIndex)
			}
			return true
		}
	}
	return t.Table.ProcessKey(e)
}

func (t *TreePanel) ProcessMouse(e *vtinput.InputEvent) bool { return t.Table.ProcessMouse(e) }

func (t *TreePanel) GetSelectedName() string {
	idx := t.cursorIndex()
	if idx < 0 {
		return ""
	}
	return t.items[idx].name
}

// activateSelected changes the source panel's directory to the highlighted
// row and closes the tree. pf.ToggleAltPanel("tree", nil) is safe here: it
// only ever calls its factory argument on the branch that opens a new tree,
// never on the branch that closes an existing one by Kind, which is the
// only branch a tree panel that is already open and handling its own Enter
// key can ever reach.
func (t *TreePanel) activateSelected() {
	idx := t.cursorIndex()
	if idx < 0 || t.src == nil {
		return
	}
	path := t.items[idx].path
	pf := FindPanelsFrameAnyScreen()
	if pf == nil {
		return
	}
	pf.NavigateToPath(t.src, path)
	pf.ToggleAltPanel("tree", nil)
}

func (t *TreePanel) Show(scr *vtui.ScreenBuf) {
	t.Frame.Show(scr)
	t.Table.Show(scr)
}

// Close is a no-op: NewTreePanel builds the tree synchronously and starts no
// background work, unlike QuickViewPanel's scan goroutine or ProcList's
// refresh ticker.
func (t *TreePanel) Close() {}
