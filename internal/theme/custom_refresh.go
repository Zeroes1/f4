package theme

import (
	"os"
	"sort"
	"strings"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
)

// AddMissingColorKeys writes into the user's farcolors.ini the colour
// elements that f4 gained after the file was written (f4#234). A Custom scheme
// is a complete export; a release that adds an element leaves it silently
// incomplete, and the new colour then comes from the base style where the
// user cannot see it, let alone change it. The lines added are the colours
// the palette holds now, so nothing on screen changes; the user's own lines,
// comments and order are not touched. It returns how many keys it added.
//
// It must run with the palette just built from the file (base style first,
// then the file), before anything adjusts it, and only for the Custom scheme:
// the palette of another style says nothing about what belongs in this file.
func AddMissingColorKeys(path string, file *ini.File) int {
	var missing []ColorSlot
	for _, slot := range ColorSlots {
		// These two are opt-in surfaces: absent means "not chosen".
		if slot.Index == vtui.ColDialogIndicatorBackground || slot.Index == ColDialogSettingsBackground {
			continue
		}
		if !colorIniDefinesSlot(file, slot) {
			missing = append(missing, slot)
		}
	}
	if len(missing) == 0 {
		return 0
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	text := string(data)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	// The end of the [farcolors] section: just after its last non-blank line.
	start := -1
	end := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[") {
			continue
		}
		if strings.EqualFold(trimmed, "[farcolors]") {
			start = i
			continue
		}
		if start >= 0 {
			end = i
			break
		}
	}
	if start < 0 {
		return 0
	}
	insertAt := end
	for insertAt > start+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	var block []string
	for _, group := range ColorGroups {
		var slots []ColorSlot
		for _, slot := range missing {
			if slot.Group == group {
				slots = append(slots, slot)
			}
		}
		if len(slots) == 0 {
			continue
		}
		sort.Slice(slots, func(i, j int) bool { return slots[i].Canonical < slots[j].Canonical })
		block = append(block, "", "# "+group+": added by a newer f4")
		for _, slot := range slots {
			block = append(block, slot.Canonical+" = "+exportSlotValue(slot))
		}
	}
	if len(block) == 0 {
		return 0
	}

	out := append(append(append([]string{}, lines[:insertAt]...), block...), lines[insertAt:]...)
	if err := os.WriteFile(path, []byte(strings.Join(out, newline)), 0600); err != nil {
		return 0
	}
	return len(missing)
}
