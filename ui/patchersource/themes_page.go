package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// promptSaveAsNewTheme asks for a name, then snapshots the live ActiveTheme
// folder (config.json + all images) into themes/<name>/ so it appears in the
// Themes picker and can be shared. The "author" field is stamped from the
// configured display name (Settings → Appearance) so other people see who
// made it when they install the theme.
func promptSaveAsNewTheme() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("e.g. Aqua, MyDark, Sunset")

	form := dialog.NewForm("Save as New Theme",
		"Save", "Cancel",
		[]*widget.FormItem{
			{Text: "Theme name", Widget: entry},
		},
		func(ok bool) {
			if !ok {
				return
			}
			name := strings.TrimSpace(entry.Text)
			if err := saveAsNewTheme(name); err != nil {
				notifyError(err)
				return
			}
			toast(toastOpts{level: modalSuccess, message: "Saved as: " + name})
		},
		mainWindow,
	)
	form.Show()
}

func saveAsNewTheme(name string) error {
	if name == "" {
		return fmt.Errorf("theme name cannot be empty")
	}
	if !isSafeThemeName(name) {
		return fmt.Errorf("theme name must be letters, digits, spaces, dashes or underscores")
	}
	if name == activeThemeDir {
		return fmt.Errorf("can't use the reserved name '%s'", activeThemeDir)
	}
	dst := filepath.Join(themesRoot, name)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("a theme named '%s' already exists — pick a different name", name)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	// Snapshot the live editor state (the ActiveTheme folder) into the new
	// theme folder. We copy ALL files so referenced images come along.
	entries, err := os.ReadDir(themeDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == "config.json" {
			continue // written separately below with overridden metadata
		}
		if err := copyFile(filepath.Join(themeDir, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return fmt.Errorf("copy %s: %w", e.Name(), err)
		}
	}

	// Snapshot the in-memory theme with overridden metadata. Using the in-memory
	// activeTheme rather than re-reading the file means we capture any edits
	// the user hasn't hit Save on yet — matching the intuitive "save the thing
	// I see right now" behaviour.
	snapshot := activeTheme
	snapshot.Name = name
	snapshot.Author = strings.TrimSpace(appSettings.UserName)
	if snapshot.Author == "" {
		snapshot.Author = "anonymous"
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, "config.json"), data, 0644)
}

func isSafeThemeName(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ' ' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// themesRoot is the folder holding all installed themes — each subfolder is one
// theme (config.json + bg image + optional icon.jpg). The "ActiveTheme" subdir
// is the live working copy and is not shown in the picker.
// Assigned by initDataPaths() at startup → <UserConfigDir>/osu-patcher/themes.
var themesRoot string

const activeThemeDir = "ActiveTheme"

type installedTheme struct {
	Dir    string // absolute path to themes/<name>/
	Folder string // bare folder name
	Cfg    ThemeConfig
}

// listInstalledThemes scans themes/ for subdirs containing config.json,
// skipping the live ActiveTheme working copy. Sorted: Default first, then
// preset themes alphabetical.
func listInstalledThemes() []installedTheme {
	entries, err := os.ReadDir(themesRoot)
	if err != nil {
		return nil
	}
	var out []installedTheme
	for _, e := range entries {
		if !e.IsDir() || e.Name() == activeThemeDir {
			continue
		}
		dir := filepath.Join(themesRoot, e.Name())
		cfgPath := filepath.Join(dir, "config.json")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			continue
		}
		var cfg ThemeConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		out = append(out, installedTheme{Dir: dir, Folder: e.Name(), Cfg: cfg})
	}
	sort.Slice(out, func(i, j int) bool {
		// "Default" first, rest alphabetical by displayed name
		if out[i].Folder == "Default" {
			return true
		}
		if out[j].Folder == "Default" {
			return false
		}
		return out[i].Cfg.Name < out[j].Cfg.Name
	})
	return out
}

// applyInstalledTheme copies every file from themes/<folder>/ into
// themes/ActiveTheme/ (so the image paths inside config.json resolve), then
// reloads the active theme and re-renders the launcher.
func applyInstalledTheme(folder string) error {
	src := filepath.Join(themesRoot, folder)
	if err := os.MkdirAll(themeDir, 0755); err != nil {
		return err
	}

	// Wipe the live slot first so removed images don't linger.
	if existing, err := os.ReadDir(themeDir); err == nil {
		for _, e := range existing {
			_ = os.Remove(filepath.Join(themeDir, e.Name()))
		}
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(themeDir, e.Name())); err != nil {
			return fmt.Errorf("copy %s: %w", e.Name(), err)
		}
	}

	if err := loadConfig(); err != nil {
		return err
	}
	buildCanvasObjects(isEditorMode)
	refreshLayersList()
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func buildThemesPage() fyne.CanvasObject {
	currentName := activeTheme.Name

	intro := canvas.NewText("Pick a preloaded design or stick with your own custom one. Applying overwrites the current Active Theme — your in-editor work will be replaced.", colorTextMute)
	intro.TextSize = 11

	// Where-on-disk card: drop themes here, share themes from here.
	pathTxt := canvas.NewText(themesRoot, colorTextStrng)
	pathTxt.TextSize = 11
	openBtn := widget.NewButtonWithIcon("Open folder", theme.FolderOpenIcon(), func() {
		if err := openInFileManager(themesRoot); err != nil {
			notifyError(err)
		}
	})
	locationCard := subCard(container.NewVBox(
		fieldLabel("THEMES FOLDER"),
		widget.NewLabel("Drop downloaded theme folders here. Your saved themes also live here — share them by zipping the folder."),
		pathTxt,
		container.NewHBox(openBtn),
	))

	var cards []fyne.CanvasObject
	cards = append(cards, intro, locationCard, widget.NewSeparator())

	themes := listInstalledThemes()
	if len(themes) == 0 {
		cards = append(cards, widget.NewLabel("No themes installed yet. Use the editor's “Save as New Theme” to make one."))
	}
	for _, t := range themes {
		t := t
		isActive := t.Cfg.Name == currentName
		cards = append(cards, themeCard(t, isActive))
	}

	return container.NewVBox(cards...)
}

func themeCard(t installedTheme, isActive bool) fyne.CanvasObject {
	// Title row: name + author
	nameTxt := canvas.NewText(t.Cfg.Name, colorTextStrng)
	nameTxt.TextSize = 18
	nameTxt.TextStyle = fyne.TextStyle{Bold: true}

	author := t.Cfg.Author
	if author == "" {
		author = "—"
	}
	authorTxt := canvas.NewText("made by "+author, colorAccent)
	authorTxt.TextSize = 11
	authorTxt.TextStyle = fyne.TextStyle{Italic: true}

	titleBlock := container.NewVBox(nameTxt, authorTxt)

	// Description
	descLbl := widget.NewLabel(t.Cfg.Description)
	descLbl.Wrapping = fyne.TextWrapWord

	// Preview thumbnail — use icon.jpg if present, else bg.jpg.
	var thumb fyne.CanvasObject
	for _, candidate := range []string{"icon.jpg", "bg.jpg", "icon.png", "bg.png"} {
		p := filepath.Join(t.Dir, candidate)
		if _, err := os.Stat(p); err == nil {
			img := canvas.NewImageFromFile(p)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(96, 96))
			thumb = img
			break
		}
	}
	if thumb == nil {
		ph := canvas.NewRectangle(colorPanel)
		ph.SetMinSize(fyne.NewSize(96, 96))
		thumb = ph
	}

	// Apply button
	applyBtn := widget.NewButtonWithIcon("Apply", theme.ConfirmIcon(), func() {
		confirmModal("Apply theme",
			fmt.Sprintf("Apply '%s'? Your current Active Theme will be replaced — any unsaved editor work is lost.", t.Cfg.Name),
			"Apply", func() {
				if err := applyInstalledTheme(t.Folder); err != nil {
					notifyError(err)
					return
				}
				toast(toastOpts{level: modalSuccess, message: "Theme: " + t.Cfg.Name})
				buildSettingsScreen() // re-render so the active badge moves
			})
	})
	applyBtn.Importance = widget.HighImportance

	var actions fyne.CanvasObject
	if isActive {
		activeTag := canvas.NewText("✓ ACTIVE", color.NRGBA{R: 80, G: 215, B: 130, A: 255})
		activeTag.TextSize = 11
		activeTag.TextStyle = fyne.TextStyle{Bold: true}
		actions = container.NewVBox(activeTag, applyBtn)
	} else {
		actions = container.NewVBox(applyBtn)
	}

	rightCol := container.NewVBox(titleBlock, descLbl, actions)
	body := container.NewBorder(nil, nil, container.NewPadded(thumb), nil, container.NewPadded(rightCol))

	return subCard(body)
}
