package dialog

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
)

func ListAvailableHelpLanguages() []i18n.Language {
	langs := []i18n.Language{{Code: "en", Name: "English"}}

	exeDir := filepath.Dir(os.Args[0])
	userDir := filepath.Join(config.GetF4ConfigDir(), "help")
	dirs := []string{filepath.Join(exeDir, "help"), userDir, "help"}
	seen := map[string]bool{"en": true}

	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".hlf") {
				code := strings.TrimSuffix(e.Name(), ".hlf")
				if !seen[code] {
					name := i18n.LanguageName(code, filepath.Join(config.GetF4ConfigDir(), "lang"))
					langs = append(langs, i18n.Language{Code: code, Name: name})
					seen[code] = true
				}
			}
		}
	}
	return langs
}
