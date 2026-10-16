package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	zipperarchive "github.com/unxed/zipper/archive"
)

// addArchiveFormat is one format the Add to archive window offers (f4#2002).
type addArchiveFormat struct {
	Label  string
	Ext    string
	Method string // the compression of a tar format; "" when the method is chosen
	// MethodName is shown for a format whose method cannot be chosen.
	MethodName string
	// Methods are the methods the user may choose between, the first one the
	// default; nil when the format has one method only.
	Methods  []addArchiveMethod
	Level    bool // the engine honours a compression level for this format
	Solid    bool // the format can be solid
	Password bool // the format can be encrypted with a password
	// Split: the archive can be cut into volumes that f4 opens again from
	// the first one. EngineSplits: the archiver writes them itself
	// (SplitSize); the other formats are cut after they are written.
	Split        bool
	EngineSplits bool
}

type addArchiveMethod struct {
	Label string
	ID    string // archive.Options.Method
}

var addArchiveFormats = []addArchiveFormat{
	{Label: "Zip", Ext: ".zip", Level: true, Password: true, Split: true, EngineSplits: true, Methods: []addArchiveMethod{
		{"Deflate", "deflate"}, {"Zstandard", "zstd"}, {"LZMA", "lzma"}, {"", "store"},
	}},
	{Label: "7z", Ext: ".7z", MethodName: "LZMA2", Solid: true, Password: true, Split: true},
	{Label: "tar + gzip", Ext: ".tar.gz", Method: "gzip", MethodName: "gzip", Level: true},
	{Label: "tar + bzip2", Ext: ".tar.bz2", Method: "bzip2", MethodName: "bzip2", Level: true},
	{Label: "tar + xz", Ext: ".tar.xz", Method: "xz", MethodName: "xz", Level: true},
}

// addArchiveLevels are the compression levels offered, 0 being the engine's
// default.
var addArchiveLevels = []struct {
	Key   string
	Level int
}{
	{"Archive.Add.LevelDefault", 0},
	{"Archive.Add.LevelFastest", 1},
	{"Archive.Add.LevelFast", 3},
	{"Archive.Add.LevelNormal", 5},
	{"Archive.Add.LevelMaximum", 7},
	{"Archive.Add.LevelUltra", 9},
}

// addArchiveSettings is what the Add to archive window was told.
type addArchiveSettings struct {
	Name   string
	Format int // index in addArchiveFormats
	Level  int // index in addArchiveLevels
	Method int // index in the format's Methods
	Solid  bool
	// Password encrypts the files of the archive (WinZip AES for zip, AES for
	// 7z); empty for none.
	Password string
	// VolumeSize cuts the archive into volumes of that many bytes, named
	// archive.zip.001, .002 and so on; 0 for one file (f4#2002, part 3).
	VolumeSize int64
}

// postSplit reports whether the archive has to be cut into volumes after
// it is written, because its archiver cannot write volumes itself.
func (s addArchiveSettings) postSplit() bool {
	f := addArchiveFormats[s.Format]
	return s.VolumeSize > 0 && f.Split && !f.EngineSplits
}

// archiveFiles is the file the archive is written to, or its first volume:
// what the panel cursor lands on and what an overwrite question is about.
func (s addArchiveSettings) archiveFile(name string) string {
	if s.VolumeSize > 0 && addArchiveFormats[s.Format].Split {
		return name + ".001"
	}
	return name
}

// addArchiveVolumeSizes are offered in the volume size field; any size can
// be typed instead.
var addArchiveVolumeSizes = []string{"10M", "100M", "650M", "700M", "1G", "4092M", "4480M"}

// parseVolumeSize reads a volume size such as 700M, 4G, 1500K or 123456
// (bytes); K, M and G are powers of 1024.
func parseVolumeSize(text string) (int64, error) {
	t := strings.ToUpper(strings.TrimSpace(text))
	t = strings.TrimSuffix(strings.TrimSuffix(t, "B"), "I")
	mult := int64(1)
	if t != "" {
		switch t[len(t)-1] {
		case 'K':
			mult = 1 << 10
		case 'M':
			mult = 1 << 20
		case 'G':
			mult = 1 << 30
		}
		if mult > 1 {
			t = strings.TrimSpace(t[:len(t)-1])
		}
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil || n <= 0 || n > (1<<62)/mult {
		return 0, fmt.Errorf("%s", vtui.Msg("Archive.Add.VolumeSizeInvalid"))
	}
	return n * mult, nil
}

// options turns the settings into what the archiver takes. Controls the
// format does not use are left out, so that a value set for one format and
// left behind when switching to another does not reach the engine.
func (s addArchiveSettings) options() zipperarchive.Options {
	f := addArchiveFormats[s.Format]
	opts := zipperarchive.Options{Xattrs: false, Method: f.Method}
	if len(f.Methods) > 0 && s.Method >= 0 && s.Method < len(f.Methods) {
		opts.Method = f.Methods[s.Method].ID
	}
	if f.Level && s.Level >= 0 && s.Level < len(addArchiveLevels) {
		opts.Level = addArchiveLevels[s.Level].Level
	}
	if f.Solid {
		opts.Solid = s.Solid
		opts.NonSolid = !s.Solid
	}
	if f.Password {
		opts.Password = s.Password
	}
	if f.Split && f.EngineSplits {
		opts.SplitSize = s.VolumeSize
	}
	return opts
}

// addArchiveFormatOf is the format whose extension name ends with, or -1.
func addArchiveFormatOf(name string) int {
	lower := strings.ToLower(name)
	best, bestLen := -1, 0
	for i, f := range addArchiveFormats {
		if strings.HasSuffix(lower, f.Ext) && len(f.Ext) > bestLen {
			best, bestLen = i, len(f.Ext)
		}
	}
	return best
}

// archiveNameForFormat gives name the extension of format, replacing the
// extension of another offered format if it has one.
func archiveNameForFormat(name string, format int) string {
	if old := addArchiveFormatOf(name); old >= 0 {
		name = name[:len(name)-len(addArchiveFormats[old].Ext)]
	}
	return name + addArchiveFormats[format].Ext
}

// addArchiveDialog is the Add to archive window: the archive name, its
// format, compression level and method, and solid where the format has it.
type addArchiveDialog struct {
	dlg    *vtui.Window
	name   *vtui.Edit
	format *vtui.ComboBox
	level  *vtui.ComboBox
	method *vtui.ComboBox
	solid  *vtui.Checkbox
	// password and showPassword: the archive password and the box that
	// shows it in clear (f4#2002, part 2).
	password     *vtui.Edit
	showPassword *vtui.Checkbox
	// split and volumeSize: cutting the archive into volumes (part 3).
	split      *vtui.Checkbox
	volumeSize *vtui.ComboBox
	create     *vtui.Button
}

func newAddArchiveDialog(initialName string, done func(addArchiveSettings)) *addArchiveDialog {
	const w, h = 64, 19
	d := &addArchiveDialog{}
	d.dlg = vtui.NewCenteredDialog(w, h, vtui.Msg("Archive.Add.Title"))
	d.dlg.ShowClose = true
	x, y := d.dlg.X1+2, d.dlg.Y1+2
	inner := w - 4

	d.name = vtui.NewEdit(x, y+1, inner, initialName)
	d.dlg.AddItem(vtui.NewLabel(x, y, vtui.Msg("Archive.Add.Name"), d.name))
	d.dlg.AddItem(d.name)

	formats := make([]string, len(addArchiveFormats))
	for i, f := range addArchiveFormats {
		formats[i] = f.Label
	}
	levels := make([]string, len(addArchiveLevels))
	for i, l := range addArchiveLevels {
		levels[i] = vtui.Msg(l.Key)
	}

	const labelW, comboW = 12, 20
	d.format = newAddArchiveCombo(x+labelW, y+3, comboW, formats)
	d.level = newAddArchiveCombo(x+labelW, y+4, comboW, levels)
	d.method = newAddArchiveCombo(x+labelW, y+5, comboW, nil)
	d.solid = vtui.NewCheckbox(x+labelW, y+6, vtui.Msg("Archive.Add.Solid"), true)
	for _, row := range []struct {
		key   string
		combo *vtui.ComboBox
		y     int
	}{{"Archive.Add.Format", d.format, y + 3}, {"Archive.Add.Level", d.level, y + 4}, {"Archive.Add.Method", d.method, y + 5}} {
		d.dlg.AddItem(vtui.NewLabel(x, row.y, vtui.Msg(row.key), row.combo))
		d.dlg.AddItem(row.combo)
	}
	d.dlg.AddItem(d.solid)

	d.password = vtui.NewPasswordEdit(x+labelW, y+8, inner-labelW, "")
	d.dlg.AddItem(vtui.NewLabel(x, y+8, vtui.Msg("Archive.Add.Password"), d.password))
	d.dlg.AddItem(d.password)
	d.showPassword = vtui.NewCheckbox(x+labelW, y+9, vtui.Msg("Archive.Add.ShowPassword"), false)
	d.showPassword.OnChange = func(state int) { d.password.PasswordMode = state != 1 }
	d.dlg.AddItem(d.showPassword)

	d.split = vtui.NewCheckbox(x, y+11, vtui.Msg("Archive.Add.Split"), false)
	d.volumeSize = vtui.NewComboBox(x+labelW+16, y+11, 12, addArchiveVolumeSizes)
	d.volumeSize.Edit.SetText("700M")
	d.volumeSize.SetDisabled(true)
	d.split.OnChange = func(state int) { d.volumeSize.SetDisabled(state != 1) }
	d.dlg.AddItem(d.split)
	d.dlg.AddItem(d.volumeSize)

	d.create = vtui.NewButton(d.dlg.X1+18, d.dlg.Y2-2, vtui.Msg("Archive.Add.Create"))
	d.create.IsDefault = true
	cancel := vtui.NewButton(d.dlg.X1+34, d.dlg.Y2-2, vtui.Msg("vtui.Cancel"))
	d.dlg.AddItem(d.create)
	d.dlg.AddItem(cancel)

	format := addArchiveFormatOf(initialName)
	if format < 0 {
		format = 0
		d.name.SetText(archiveNameForFormat(initialName, format))
	}
	d.selectFormat(format)
	d.format.Menu.OnAction = func(idx int) {
		d.name.SetText(archiveNameForFormat(d.name.GetText(), idx))
		d.selectFormat(idx)
	}

	d.create.OnClick = func() {
		settings := d.settings()
		if strings.TrimSpace(settings.Name) == "" {
			return
		}
		if d.split.State == 1 {
			size, err := parseVolumeSize(d.volumeSize.Edit.GetText())
			if err != nil {
				vtui.ShowMessageEx(vtui.Msg("Archive.Add.Title"), err.Error(), []string{vtui.Msg("vtui.Ok")}, vtui.MessageWarn)
				return
			}
			settings.VolumeSize = size
		}
		d.dlg.Close()
		done(settings)
	}
	cancel.OnClick = func() { d.dlg.Close() }
	d.dlg.SetFocusedItem(d.name)
	return d
}

func newAddArchiveCombo(x, y, w int, items []string) *vtui.ComboBox {
	c := vtui.NewComboBox(x, y, w, items)
	c.DropdownOnly = true
	return c
}

func setAddArchiveCombo(c *vtui.ComboBox, idx int) {
	c.Menu.SetSelectPos(idx)
	if idx >= 0 && idx < len(c.Menu.Items) {
		c.Edit.SetText(c.Menu.Items[idx].Text)
	}
}

// selectFormat shows format as chosen and offers only the controls it uses.
func (d *addArchiveDialog) selectFormat(idx int) {
	f := addArchiveFormats[idx]
	setAddArchiveCombo(d.format, idx)

	d.method.Menu.Items = nil
	switch {
	case len(f.Methods) > 0:
		for _, m := range f.Methods {
			label := m.Label
			if label == "" {
				label = vtui.Msg("Archive.Add.MethodStore")
			}
			d.method.Menu.AddItem(vtui.MenuItem{Text: label})
		}
	default:
		d.method.Menu.AddItem(vtui.MenuItem{Text: f.MethodName})
	}
	setAddArchiveCombo(d.method, 0)
	d.method.SetDisabled(len(f.Methods) == 0)

	level := d.level.Menu.SelectPos
	if !f.Level || level < 0 {
		level = 0
	}
	setAddArchiveCombo(d.level, level)
	d.level.SetDisabled(!f.Level)

	// Solid is on by default where the format has it, and shown off where
	// it does not.
	if f.Solid {
		d.solid.State = 1
	} else {
		d.solid.State = 0
	}
	d.solid.SetDisabled(!f.Solid)

	// A format without encryption takes no password: one typed for another
	// format is dropped rather than silently ignored at archiving time.
	if !f.Password {
		d.password.SetText("")
	}
	d.password.SetDisabled(!f.Password)
	d.showPassword.SetDisabled(!f.Password)

	// Volumes of a tar archive are not opened again from the first one
	// yet, so a tar format is written as one file.
	if !f.Split {
		d.split.State = 0
	}
	d.split.SetDisabled(!f.Split)
	d.volumeSize.SetDisabled(!f.Split || d.split.State != 1)
}

func (d *addArchiveDialog) settings() addArchiveSettings {
	return addArchiveSettings{
		Name:     strings.TrimSpace(d.name.GetText()),
		Format:   d.format.Menu.SelectPos,
		Level:    d.level.Menu.SelectPos,
		Method:   d.method.Menu.SelectPos,
		Solid:    d.solid.State == 1,
		Password: d.password.GetText(),
	}
}

// showAddArchiveDialog opens the Add to archive window; a variable so that
// tests can answer it.
var showAddArchiveDialog = func(_ vfs.App, initialName string, done func(addArchiveSettings)) {
	if vtui.FrameManager == nil {
		return
	}
	vtui.FrameManager.Push(newAddArchiveDialog(initialName, done).dlg)
}

// splitIntoVolumes cuts the archive at path into volumes of size bytes,
// path.001, path.002 and so on, the byte split 7-Zip makes with -v and
// archive.OpenInput reads back, then removes path. A failure leaves path as
// it was and removes the volumes already written.
func splitIntoVolumes(ctx context.Context, path string, size int64) (retErr error) {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	var written []string
	defer func() {
		_ = src.Close() // Read-only; nothing to flush.
		if retErr != nil {
			for _, v := range written {
				_ = os.Remove(v) // Best effort: the archive itself is kept.
			}
		}
	}()
	for i := 1; ; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := fmt.Sprintf("%s.%03d", path, i)
		dst, err := os.Create(name)
		if err != nil {
			return err
		}
		written = append(written, name)
		n, copyErr := io.CopyN(dst, src, size)
		closeErr := dst.Close()
		if copyErr != nil && copyErr != io.EOF {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if copyErr == io.EOF {
			if n == 0 && i > 1 {
				// The archive ended exactly on a volume boundary.
				written = written[:len(written)-1]
				if err := os.Remove(name); err != nil {
					return err
				}
			}
			break
		}
	}
	_ = src.Close() // Closed before the remove for Windows; the defer's second close is harmless.
	return os.Remove(path)
}
