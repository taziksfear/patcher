package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openInFileManager opens the OS native file browser at path.
//   Linux:   xdg-open
//   Windows: explorer
//   macOS:   open
func openInFileManager(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// Preset themes are embedded into the binary so they survive a clean install
// — the user data dir gets seeded from this on first run (and any missing
// presets are re-extracted on later runs). User-edited Default + any
// "Save as New Theme" outputs live only on disk and are never overwritten.
//
//go:embed themes/Kurumi themes/Astolfo themes/Nakuru
var presetThemesFS embed.FS

// All UI strings live in localisation.json. Embedding it keeps translations
// shippable in a single binary while staying editable as plain JSON in source.
// See loadTranslations() in main.go.
//
//go:embed localisation.json
var embeddedLocalisationJSON []byte

// dataRoot returns the per-user config dir for this app, falling back to
// the working dir if the OS doesn't expose one (very unusual).
//   Linux:   ~/.config/osu-patcher
//   Windows: %AppData%\osu-patcher
//   macOS:   ~/Library/Application Support/osu-patcher
func dataRoot() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		// Last-resort fallback so the app can still start; we'll just write
		// next to the binary. Better than crashing.
		base = "."
	}
	return filepath.Join(base, "osu-patcher")
}

// initDataPaths sets the package-level path vars to the per-user data dir,
// creates the directory tree, runs a one-shot migration from any legacy
// ./themes + ./settings.json next to the binary, and seeds the preset themes.
func initDataPaths() {
	root := dataRoot()
	themesRoot = filepath.Join(root, "themes")
	themeDir = filepath.Join(themesRoot, activeThemeDir)
	settingsPath = filepath.Join(root, "settings.json")

	if err := os.MkdirAll(themeDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[data] cannot create %s: %v\n", themeDir, err)
	}

	migrateLegacyData(root)
	seedPresetThemes()
}

// migrateLegacyData moves ./themes and ./settings.json (next to the binary,
// the old location) into the new per-user dir on first run. Only fires when
// the legacy data exists AND the new dir is empty, so it never overwrites
// anything.
func migrateLegacyData(root string) {
	// settings.json
	if !fileExists(filepath.Join(root, "settings.json")) && fileExists("settings.json") {
		if data, err := os.ReadFile("settings.json"); err == nil {
			_ = os.WriteFile(filepath.Join(root, "settings.json"), data, 0644)
			fmt.Printf("[data] migrated legacy settings.json → %s\n", root)
		}
	}
	// themes/<*>/  — only migrate if user-side themes dir has nothing
	if entries, err := os.ReadDir("themes"); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			src := filepath.Join("themes", e.Name())
			dst := filepath.Join(themesRoot, e.Name())
			if dirExists(dst) {
				continue
			}
			if err := copyDir(src, dst); err == nil {
				fmt.Printf("[data] migrated theme '%s' → %s\n", e.Name(), dst)
			}
		}
	}
}

// seedPresetThemes copies embedded preset themes into the user data dir if
// they're missing. Re-runs every startup so deleting a preset folder restores
// it on next launch — keeps a "factory reset" path always available for the
// presets without nuking user themes.
func seedPresetThemes() {
	root, err := fs.Sub(presetThemesFS, "themes")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[data] embed root missing: %v\n", err)
		return
	}
	presets, err := fs.ReadDir(root, ".")
	if err != nil {
		return
	}
	for _, p := range presets {
		if !p.IsDir() {
			continue
		}
		dst := filepath.Join(themesRoot, p.Name())
		if dirExists(dst) {
			continue
		}
		if err := extractEmbeddedDir(root, p.Name(), dst); err != nil {
			fmt.Fprintf(os.Stderr, "[data] seed %s: %v\n", p.Name(), err)
			continue
		}
		fmt.Printf("[data] seeded preset theme '%s' → %s\n", p.Name(), dst)
	}
}

func extractEmbeddedDir(root fs.FS, name, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := fs.ReadDir(root, name)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue // presets are flat — no nested dirs
		}
		data, err := fs.ReadFile(root, filepath.ToSlash(filepath.Join(name, e.Name())))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0644); err != nil {
			return err
		}
	}
	return nil
}
