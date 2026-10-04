package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// translations is populated from the embedded localisation.json at startup
// by loadTranslations() (see data_paths.go for the //go:embed directive).
var translations = map[string]map[string]string{}

func loadTranslations() {
	if err := json.Unmarshal(embeddedLocalisationJSON, &translations); err != nil {
		fmt.Fprintf(os.Stderr, "[i18n] failed to parse localisation.json: %v\n", err)
		translations = map[string]map[string]string{"English": {}}
	}
}

// availableLanguages returns language keys with English first, then the rest
// alphabetical — so the language picker order is stable as new translations
// get added to the JSON.
func availableLanguages() []string {
	out := make([]string, 0, len(translations))
	hasEnglish := false
	for k := range translations {
		if k == "English" {
			hasEnglish = true
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	if hasEnglish {
		out = append([]string{"English"}, out...)
	}
	return out
}

func T(key string) string {
	lang := appSettings.Language
	if lang == "" {
		lang = "English"
	}
	if val, ok := translations[lang][key]; ok && val != "" {
		return val
	}
	// Fall back to English so a missing key in a partial translation doesn't
	// surface the raw machine key (e.g. "menu_file") to the user.
	if val, ok := translations["English"][key]; ok && val != "" {
		return val
	}
	return key
}

type UIElement struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Text         string  `json:"text,omitempty"`
	Action       string  `json:"action,omitempty"`
	X            float32 `json:"x"`
	Y            float32 `json:"y"`
	Width        float32 `json:"width,omitempty"`
	Height       float32 `json:"height,omitempty"`
	FontSize     float32 `json:"font_size,omitempty"`
	Opacity      float64 `json:"opacity"`
	Blur         float64 `json:"blur,omitempty"`
	Image        string  `json:"image,omitempty"`
	Endpoint     string  `json:"endpoint,omitempty"`
	ColorR       uint8   `json:"color_r"`
	ColorG       uint8   `json:"color_g"`
	ColorB       uint8   `json:"color_b"`
	TextColorR   uint8   `json:"text_color_r"`
	TextColorG   uint8   `json:"text_color_g"`
	TextColorB   uint8   `json:"text_color_b"`
	ModuleMode   string  `json:"module_mode,omitempty"`
	CornerRadius float32 `json:"corner_radius,omitempty"`
}

func (e *UIElement) Radius(defaultR float32) float32 {
	if e.CornerRadius > 0 {
		return e.CornerRadius
	}
	return defaultR
}

type ThemeConfig struct {
	Name        string       `json:"name"`
	Author      string       `json:"author,omitempty"`
	Description string       `json:"description,omitempty"`
	Elements    []*UIElement `json:"elements"`
}

type AppSettings struct {
	Language      string `json:"language"`
	UserName      string `json:"user_name"` // displayed as the author on themes the user creates
	GamePath      string `json:"game_path"` // legacy, kept for one cycle so old settings.json files load cleanly; auto-migrates into OsuFolder
	PatcherPath   string `json:"patcher_path"`
	OsuFolder     string `json:"osu_folder"` // the user's osu! install — the folder that holds their osu!.exe
	Server        string `json:"server"`     // -devserver value (e.g. "akatsuki.gg", "bancho")
	TopPlaysURL   string `json:"top_plays_url"`
	LaunchCommand string `json:"launch_command"`
}

type ServerPreset struct {
	Name        string // human label
	DevServer   string // -devserver value / server.txt content
	Description string
}

var serverPresets = []ServerPreset{
	{"Bancho (official)", "bancho", "official osu! server, no injection"},
	{"Akatsuki", "akatsuki.gg", "popular relax/auto server"},
	{"Ripple", "ripple.moe", "classic private server"},
	{"Gatari", "gatari.pw", "private server"},
	{"Datenshi", "datenshi.xyz", "private server"},
	{"Kawata", "kawata.pw", "private server"},
}

var (
	mainWindow   fyne.Window
	currentApp   fyne.App
	activeTheme  ThemeConfig
	appSettings  AppSettings
	isEditorMode bool

	liveCanvas     *fyne.Container
	inspectorPanel *fyne.Container
	layersList     *widget.List

	selectedElement *UIElement

	// These are assigned at startup by initDataPaths() so they resolve to the
	// per-user config directory (~/.config/osu-patcher on Linux,
	// %AppData%\osu-patcher on Windows) rather than the working directory.
	themeDir     string
	settingsPath string
)

var (
	colorTopBar    = color.NRGBA{R: 10, G: 9, B: 14, A: 255}
	colorSidebar   = color.NRGBA{R: 16, G: 14, B: 22, A: 255}
	colorSettings  = color.NRGBA{R: 12, G: 11, B: 18, A: 255}
	colorPanel     = color.NRGBA{R: 22, G: 20, B: 30, A: 255}
	colorPanelHi   = color.NRGBA{R: 34, G: 28, B: 48, A: 255}
	colorAccent    = color.NRGBA{R: 255, G: 102, B: 153, A: 255}
	colorTextMute  = color.NRGBA{R: 175, G: 170, B: 200, A: 220}
	colorTextStrng = color.NRGBA{R: 250, G: 248, B: 255, A: 255}
	colorOk        = color.NRGBA{R: 80, G: 215, B: 130, A: 255}
	colorDanger    = color.NRGBA{R: 255, G: 80, B: 100, A: 255}
	colorWarn      = color.NRGBA{R: 245, G: 190, B: 70, A: 255}
)

func loadAppSettings() {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		appSettings = AppSettings{Language: "English", OsuFolder: detectOsuFolder()}
		return
	}
	json.Unmarshal(data, &appSettings)

	before := appSettings

	// Migration: the Game settings page used to write to GamePath (the path to
	// osu!.exe). The launch pipeline only ever reads OsuFolder. Anyone whose
	// last action was on the now-removed Game page would otherwise be silently
	// unable to launch. Promote the dir part of GamePath into OsuFolder so it
	// just works on next start, then clear the dead field.
	if strings.TrimSpace(appSettings.OsuFolder) == "" && strings.TrimSpace(appSettings.GamePath) != "" {
		appSettings.OsuFolder = filepath.Dir(normalizePath(appSettings.GamePath))
		appSettings.GamePath = ""
	}

	// Migration: earlier builds moved the game into <install>/real_osu and
	// pointed OsuFolder there. The game is left in place now, so walk that back
	// up to the install root — otherwise the saved path points at a folder that
	// no longer exists and Launch dead-ends on "folder not found".
	if folder := normalizePath(strings.TrimSpace(appSettings.OsuFolder)); filepath.Base(folder) == "real_osu" {
		appSettings.OsuFolder = filepath.Dir(folder)
	}

	if !osuFolderValid(appSettings.OsuFolder) {
		if detected := detectOsuFolder(); detected != "" {
			appSettings.OsuFolder = detected
		}
	}

	if appSettings != before {
		saveAppSettings()
	}
}

// osuFolderValid reports whether the configured folder actually holds a game we
// can launch. Used to decide whether auto-detection should step in, and to show
// a live status in Settings.
func osuFolderValid(folder string) bool {
	folder = normalizePath(strings.TrimSpace(folder))
	return folder != "" && fileExists(filepath.Join(folder, osuExeName))
}

// detectOsuFolder finds an existing osu! install so first-run users don't have
// to go hunting: osu-winello records the folder it manages on Linux, and the
// Windows installer defaults to %LOCALAPPDATA%\osu!.
func detectOsuFolder() string {
	var candidates []string
	if runtime.GOOS == "windows" {
		for _, env := range []string{"LOCALAPPDATA", "ProgramFiles(x86)", "ProgramFiles"} {
			if base := os.Getenv(env); base != "" {
				candidates = append(candidates, filepath.Join(base, "osu!"))
			}
		}
	} else {
		if p := osuWineManagedPath(); p != "" {
			candidates = append(candidates, p)
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, ".local", "share", "osu-wine", "osu!"))
		}
	}
	for _, c := range candidates {
		if fileExists(filepath.Join(c, osuExeName)) {
			return c
		}
	}
	return ""
}

// osuWineManagedPath returns the osu! folder osu-winello is configured to
// launch, or "" when osu-winello isn't installed.
func osuWineManagedPath() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	data, err := os.ReadFile(filepath.Join(dataHome, "osuconfig", "osupath"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveAppSettings() {
	data, _ := json.MarshalIndent(appSettings, "", "  ")
	os.WriteFile(settingsPath, data, 0644)
}

func ensureThemeDir() { os.MkdirAll(themeDir, 0755) }

func configPath() string { return filepath.Join(themeDir, "config.json") }

func saveConfig() {
	if err := writeConfig(); err != nil {
		notifyError(err)
		return
	}
	toastSaved("theme")
}

func writeConfig() error {
	ensureThemeDir()
	// Stamp the user's display name as author on first save when the slot is
	// empty. This way themes the user customizes get attributed to them
	// automatically if they later "Save as new theme" and share it.
	if activeTheme.Author == "" {
		if u := strings.TrimSpace(appSettings.UserName); u != "" {
			activeTheme.Author = u
		}
	}
	data, err := json.MarshalIndent(activeTheme, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}

func loadConfig() error {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &activeTheme); err != nil {
		return err
	}
	if dropRemovedActions() {
		return writeConfig()
	}
	return nil
}

// dropRemovedActions strips elements bound to actions that no longer exist.
// Themes authored while custom clients were still a feature carry a "switch
// client" button, which would otherwise sit there doing nothing when clicked.
func dropRemovedActions() bool {
	kept := make([]*UIElement, 0, len(activeTheme.Elements))
	for _, e := range activeTheme.Elements {
		if e.Action == "internal://switch_client" {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(activeTheme.Elements) {
		return false
	}
	activeTheme.Elements = kept
	return true
}

func copyImageToTheme(srcPath string) (string, error) {
	ensureThemeDir()
	fileName := filepath.Base(srcPath)
	dstPath := filepath.Join(themeDir, fileName)
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return fileName, nil
}

func exportToPatcherCfg() {
	saveDialog := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil || uc == nil {
			return
		}
		defer uc.Close()
		zw := zip.NewWriter(uc)
		defer zw.Close()
		entries, err := os.ReadDir(themeDir)
		if err != nil {
			notifyError(err)
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(themeDir, entry.Name()))
			if err != nil {
				continue
			}
			w, err := zw.Create(entry.Name())
			if err != nil {
				continue
			}
			w.Write(data)
		}
		toast(toastOpts{level: modalSuccess, message: T("export_msg")})
	}, mainWindow)
	saveDialog.SetFileName(activeTheme.Name + ".patchercfg")
	saveDialog.Show()
}

type DraggableWrapper struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	element  *UIElement
	onSelect func(*UIElement)
}

func NewDraggable(content fyne.CanvasObject, el *UIElement, onSelect func(*UIElement)) *DraggableWrapper {
	d := &DraggableWrapper{content: content, element: el, onSelect: onSelect}
	d.ExtendBaseWidget(d)
	return d
}

func (d *DraggableWrapper) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(d.content)
}

func (d *DraggableWrapper) Dragged(e *fyne.DragEvent) {
	if d.element.Type == "background" {
		return
	}
	d.Move(d.Position().Add(e.Dragged))
}

func (d *DraggableWrapper) DragEnd() {
	if d.element.Type == "background" {
		return
	}
	d.element.X = d.Position().X
	d.element.Y = d.Position().Y
	if selectedElement == d.element {
		showPropertiesPanel(d.element)
	}
}

func (d *DraggableWrapper) Tapped(_ *fyne.PointEvent) {
	selectedElement = d.element
	if d.onSelect != nil {
		d.onSelect(d.element)
	}
}

type tappableContainer struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	onTapped func()
}

func newTappableContainer(content fyne.CanvasObject, onTapped func()) *tappableContainer {
	t := &tappableContainer{content: content, onTapped: onTapped}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableContainer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

func (t *tappableContainer) Tapped(_ *fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func buildCanvasObjects(isEditMode bool) {
	liveCanvas.Objects = nil
	for _, el := range activeTheme.Elements {
		obj := buildElementVisual(el, isEditMode)
		if obj != nil {
			liveCanvas.Add(obj)
		}
	}
	liveCanvas.Refresh()
}

func buildElementVisual(el *UIElement, isEditMode bool) fyne.CanvasObject {
	alpha := uint8(float64(255) * clamp01(el.Opacity))

	winSize := mainWindow.Canvas().Size()
	if winSize.Width <= 0 || winSize.Height <= 0 {
		winSize = fyne.NewSize(1100, 650)
	}

	var visual fyne.CanvasObject

	switch el.Type {

	case "background":
		var bg fyne.CanvasObject
		if el.Image != "" {
			imgPath := filepath.Join(themeDir, el.Image)
			if decoded := stretched(imgPath, int(winSize.Width), int(winSize.Height)); decoded != nil {
				img := canvas.NewImageFromImage(decoded)
				img.FillMode = canvas.ImageFillStretch
				img.Translucency = 1.0 - el.Opacity
				img.Resize(winSize)
				img.Move(fyne.NewPos(0, 0))
				bg = img
			}
		}
		if bg == nil {
			r := canvas.NewRectangle(color.NRGBA{R: el.ColorR, G: el.ColorG, B: el.ColorB, A: alpha})
			r.Resize(winSize)
			r.Move(fyne.NewPos(0, 0))
			bg = r
		}
		if isEditMode {
			d := NewDraggable(bg, el, showPropertiesPanel)
			d.Move(fyne.NewPos(0, 0))
			d.Resize(winSize)
			return d
		}
		return bg

	case "text":
		txt := canvas.NewText(el.Text, color.NRGBA{
			R: el.TextColorR, G: el.TextColorG, B: el.TextColorB, A: alpha,
		})
		fs := el.FontSize
		if fs <= 0 {
			fs = 16
		}
		txt.TextSize = fs
		visual = txt

	case "button":
		bgRect := canvas.NewRectangle(color.NRGBA{R: el.ColorR, G: el.ColorG, B: el.ColorB, A: alpha})
		bgRect.CornerRadius = el.Radius(6)
		label := canvas.NewText(el.Text, color.NRGBA{
			R: el.TextColorR, G: el.TextColorG, B: el.TextColorB, A: 255,
		})
		label.TextSize = 14
		label.Alignment = fyne.TextAlignCenter
		w, h := el.Width, el.Height
		if w <= 0 {
			w = 120
		}
		if h <= 0 {
			h = 40
		}
		btnContainer := container.NewStack(bgRect, container.NewCenter(label))
		btnContainer.Resize(fyne.NewSize(w, h))
		if !isEditMode {
			tap := newTappableContainer(btnContainer, func() { handleButtonAction(el) })
			tap.Move(fyne.NewPos(el.X, el.Y))
			tap.Resize(fyne.NewSize(w, h))
			return tap
		}
		visual = btnContainer

	case "topplays":
		w, h := el.Width, el.Height
		if w <= 0 {
			w = 440
		}
		if h <= 0 {
			h = 400
		}
		tpObj := BuildTopPlaysWidget(el)
		if isEditMode {
			d := NewDraggable(tpObj, el, showPropertiesPanel)
			d.Move(fyne.NewPos(el.X, el.Y))
			d.Resize(fyne.NewSize(w, h))
			return d
		}
		tpObj.Resize(fyne.NewSize(w, h))
		tpObj.Move(fyne.NewPos(el.X, el.Y))
		return tpObj

	case "module": // api text / webview screenshot tile
		bgRect := canvas.NewRectangle(color.NRGBA{
			R: el.ColorR, G: el.ColorG, B: el.ColorB, A: alpha,
		})
		bgRect.CornerRadius = el.Radius(8)

		w, h := el.Width, el.Height
		if w <= 0 {
			w = 200
		}
		if h <= 0 {
			h = 80
		}

		var modContainer *fyne.Container

		if el.ModuleMode == "url" && el.Endpoint != "" {

			if !isEditMode {
				webImg := StartModuleWebView(el.ID, el.Endpoint, int(w), int(h))
				webImg.Resize(fyne.NewSize(w, h))
				tap := newTappableContainer(webImg, func() { OpenEmbeddedWebView(el.Endpoint, 1100, 750) })
				tap.Move(fyne.NewPos(el.X, el.Y))
				tap.Resize(fyne.NewSize(w, h))
				return tap
			}
			hostname := el.Endpoint
			if u, err := url.Parse(el.Endpoint); err == nil && u.Host != "" {
				hostname = u.Host
			}
			lbl := canvas.NewText(hostname, color.NRGBA{R: el.TextColorR, G: el.TextColorG, B: el.TextColorB, A: 255})
			lbl.TextSize = 11
			modContainer = container.NewStack(bgRect, container.NewCenter(lbl))
		} else {
			textColor := color.NRGBA{R: el.TextColorR, G: el.TextColorG, B: el.TextColorB, A: 255}

			if !isEditMode && el.Endpoint != "" {
				label := canvas.NewText("loading", textColor)
				label.TextSize = 12
				modContainer = container.NewStack(bgRect, container.NewCenter(label))

				go func(endpoint string, lbl *canvas.Text) {
					resp, err := http.Get(endpoint)
					if err != nil {
						lbl.Text = "Ошибка сети"
						lbl.Refresh()
						return
					}
					defer resp.Body.Close()
					type BeatmapData struct {
						SongName string `json:"song_name"`
					}
					type ScoreData struct {
						PP       float64     `json:"pp"`
						Accuracy float64     `json:"accuracy"`
						Rank     string      `json:"rank"`
						MaxCombo int         `json:"max_combo"`
						Beatmap  BeatmapData `json:"beatmap"`
					}
					type AkatsukiResponse struct {
						Code   int         `json:"code"`
						Scores []ScoreData `json:"scores"`
					}

					var apiRes AkatsukiResponse
					if err := json.NewDecoder(resp.Body).Decode(&apiRes); err == nil {
						if apiRes.Code != 200 {
							lbl.Text = "API Ошибка"
						} else if len(apiRes.Scores) > 0 {
							top := apiRes.Scores[0]
							lbl.Text = fmt.Sprintf("Top: %s | %.0fpp", top.Beatmap.SongName, top.PP)
						} else {
							lbl.Text = "no scores"
						}
					} else {
						lbl.Text = "bad API"
					}
					lbl.Refresh()
				}(el.Endpoint, label)

				tap := newTappableContainer(modContainer, func() { handleEndpointClick(el.Endpoint) })
				tap.Move(fyne.NewPos(el.X, el.Y))
				tap.Resize(fyne.NewSize(w, h))
				return tap
			} else {
				txt := "{ API: " + el.Endpoint + " }"
				if el.Endpoint == "" {
					txt = "{ API: empty}"
				}
				label := canvas.NewText(txt, textColor)
				label.TextSize = 12
				modContainer = container.NewStack(bgRect, container.NewCenter(label))
			}
		}

		visual = modContainer
	}
	if visual == nil {
		return nil
	}

	if isEditMode {
		d := NewDraggable(visual, el, showPropertiesPanel)
		d.Move(fyne.NewPos(el.X, el.Y))
		if el.Width > 0 && el.Height > 0 {
			d.Resize(fyne.NewSize(el.Width, el.Height))
		} else {
			d.Resize(visual.MinSize())
		}
		return d
	}

	visual.Move(fyne.NewPos(el.X, el.Y))
	if el.Width > 0 && el.Height > 0 {
		visual.Resize(fyne.NewSize(el.Width, el.Height))
	}
	return visual
}

func handleEndpointClick(endpoint string) {
	if endpoint == "" {
		return
	}

	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		u, err := url.Parse(endpoint)
		if err == nil {
			fyne.CurrentApp().OpenURL(u)
			return
		}
	}

	notify("API Module", "Endpoint: "+endpoint)
}

func handleButtonAction(el *UIElement) {
	if el.Action == "" {
		return
	}
	if strings.HasPrefix(el.Action, "http://") || strings.HasPrefix(el.Action, "https://") {
		u, err := url.Parse(el.Action)
		if err == nil {
			fyne.CurrentApp().OpenURL(u)
		}
		return
	}
	if el.Action == "internal://launch" {
		launchOsu()
		return
	}
	if el.Action == "internal://switch_server" {
		showServerPicker()
		return
	}
}

func showServerPicker() {
	currentLbl := widget.NewLabel("current: " + currentServerLabel())

	var items []fyne.CanvasObject
	items = append(items, currentLbl, widget.NewSeparator())

	var dlg dialog.Dialog

	for _, p := range serverPresets {
		p := p
		btn := widget.NewButton(p.Name+"  —  "+p.Description, func() {
			appSettings.Server = p.DevServer
			saveAppSettings()
			if dlg != nil {
				dlg.Hide()
			}
			toast(toastOpts{level: modalSuccess, message: "Server: " + p.Name})
		})
		btn.Alignment = widget.ButtonAlignLeading
		items = append(items, btn)
	}

	customEntry := widget.NewEntry()
	customEntry.SetPlaceHolder("custom -devserver value (e.g. mysrv.example.com)")
	customBtn := widget.NewButton("Use custom", func() {
		v := strings.TrimSpace(customEntry.Text)
		if v == "" {
			return
		}
		appSettings.Server = v
		saveAppSettings()
		if dlg != nil {
			dlg.Hide()
		}
		toast(toastOpts{level: modalSuccess, message: "Server: " + v})
	})
	items = append(items, widget.NewSeparator(), customEntry, customBtn)

	content := container.NewVScroll(container.NewVBox(items...))
	content.SetMinSize(fyne.NewSize(420, 360))
	dlg = dialog.NewCustom("Switch Server", "Cancel", content, mainWindow)
	dlg.Show()
}

func currentServerLabel() string {
	srv := currentServer()
	for _, p := range serverPresets {
		if p.DevServer == srv {
			return p.Name
		}
	}
	return srv
}

// ── launch / inject pipeline ────────────────────────────────────────────────
//
// The user's osu! install is never restructured: the game keeps its own folder
// and its own osu!.exe. All we ever add is osu_patcher.exe beside it.
//
//   bancho         → start the game directly, the patcher stays out of the way
//   private server → start osu_patcher.exe, which launches osu! and injects
//
// On Linux everything goes through osu-wine so its wineprefix, tablet hack and
// runtime tweaks apply either way. A user-supplied LaunchCommand overrides the
// whole pipeline.

const (
	patcherFilename = "osu_patcher.exe"
	osuExeName      = "osu!.exe"
)

// needsPatcher reports whether a server selection requires the injected runtime.
// Only private servers do; on bancho we never touch the game.
func needsPatcher(server string) bool {
	s := strings.ToLower(strings.TrimSpace(server))
	return s != "" && s != "bancho"
}

func currentServer() string {
	srv := strings.TrimSpace(appSettings.Server)
	if srv == "" {
		return "bancho"
	}
	return srv
}

// promptOsuFolderThenLaunch pops the native folder picker, saves whatever the
// user picks as the osu! folder, and immediately re-attempts launchOsu. Used
// as a "set up on the fly" path so first-time users don't get blocked by an
// error modal and never realise they need to visit Settings.
func promptOsuFolderThenLaunch() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		picked := normalizePath(uri.Path())
		if picked == "" {
			return
		}
		appSettings.OsuFolder = picked
		saveAppSettings()
		toast(toastOpts{level: modalSuccess, message: "osu! folder set"})
		launchOsu() // retry with the freshly-saved folder
	}, mainWindow)
}

func launchOsu() {
	// A custom launch command replaces the pipeline outright, so it runs before
	// any of our folder checks — it may not involve our osu! folder at all.
	if raw := strings.TrimSpace(appSettings.LaunchCommand); raw != "" {
		parts := strings.Fields(raw)
		bin, err := resolveBinary(parts[0])
		if err != nil {
			notifyError(fmt.Errorf("custom launch command: %v", err))
			return
		}
		c := exec.Command(bin, parts[1:]...)
		c.Env = os.Environ()
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Start(); err != nil {
			notifyError(fmt.Errorf("custom launch failed: %v", err))
			return
		}
		go c.Wait()
		fmt.Printf("[Launch] custom: %s (pid %d)\n", raw, c.Process.Pid)
		return
	}

	rawSetting := strings.TrimSpace(appSettings.OsuFolder)
	osuDir := normalizePath(rawSetting)

	// First-run convenience: rather than dropping a wall-of-text error on
	// someone hitting Launch on a clean install, open the folder picker
	// directly. After they pick, we save it and continue the launch.
	if osuDir == "" {
		promptOsuFolderThenLaunch()
		return
	}
	if !dirExists(osuDir) {
		showModal(modalError, "osu! folder not found",
			fmt.Sprintf("The saved folder doesn't exist on disk:\n  %s\n\n(raw value: %q)\n\nPick it again?", osuDir, rawSetting),
			modalAction{Label: "Cancel"},
			modalAction{Label: "Pick folder…", Primary: true, OnClick: promptOsuFolderThenLaunch},
		)
		return
	}

	if !fileExists(filepath.Join(osuDir, osuExeName)) {
		showModal(modalError, "osu!.exe not found",
			fmt.Sprintf("No %s in:\n  %s\n\nPick the folder your osu! is installed in — the one holding osu!.exe.", osuExeName, osuDir),
			modalAction{Label: "Cancel"},
			modalAction{Label: "Pick folder…", Primary: true, OnClick: promptOsuFolderThenLaunch},
		)
		return
	}
	finishLaunch(osuDir)
}

func finishLaunch(osuDir string) {
	srv := currentServer()

	if needsPatcher(srv) {
		if err := installPatcherExe(osuDir); err != nil {
			notifyError(fmt.Errorf("install patcher: %v", err))
			return
		}
	}

	cmd, source, err := launchCommand(osuDir, srv)
	if err != nil {
		notifyError(fmt.Errorf("cannot launch: %v", err))
		return
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		notifyError(fmt.Errorf("start %s: %v", source, err))
		return
	}
	go cmd.Wait()
	fmt.Printf("[Launch] %s (pid %d) | server=%s osuDir=%s\n", source, cmd.Process.Pid, srv, osuDir)
}

// installPatcherExe drops the injector next to the game as osu_patcher.exe.
// Falls back to the user-configured PatcherPath if this build has no bundled
// exe. An already-identical file is left alone so we don't rewrite ~32MB on
// every launch.
func installPatcherExe(osuDir string) error {
	dst := filepath.Join(osuDir, patcherFilename)

	if hasEmbeddedPatcher() {
		if fileHasContent(dst, embeddedPatcherExe) {
			return nil
		}
		return os.WriteFile(dst, embeddedPatcherExe, 0755)
	}

	src := normalizePath(strings.TrimSpace(appSettings.PatcherPath))
	if src == "" {
		return fmt.Errorf("no bundled patcher in this build and no PatcherPath set — rebuild via build.sh or set Settings → Game → Advanced")
	}
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("copy %s: %v", src, err)
	}
	return os.Chmod(dst, 0755)
}

// fileHasContent reports whether path already holds exactly want. It streams
// rather than reading the file in, because want is the ~32MB embedded patcher
// and this runs on every launch.
func fileHasContent(path string, want []byte) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() != int64(len(want)) {
		return false
	}
	buf := make([]byte, 64*1024)
	for off := 0; off < len(want); {
		n, err := io.ReadFull(f, buf[:min(len(buf), len(want)-off)])
		if err != nil || !bytes.Equal(buf[:n], want[off:off+n]) {
			return false
		}
		off += n
	}
	return true
}

// launchCommand builds the command that starts the game.
//
//	Windows: run the patcher (private server) or osu!.exe itself (bancho).
//	Linux:   go through osu-wine so the wineprefix and its runtime tweaks apply.
//	         Plain `osu-wine` is preferred when it already manages this folder,
//	         since that path also applies the user's pre/post launch args.
func launchCommand(osuDir, srv string) (*exec.Cmd, string, error) {
	patcher := filepath.Join(osuDir, patcherFilename)
	osuExe := filepath.Join(osuDir, osuExeName)
	inject := needsPatcher(srv)

	if runtime.GOOS == "windows" {
		var c *exec.Cmd
		source := osuExe
		if inject {
			c = exec.Command(patcher, "--server", srv)
			source = patcher
		} else {
			c = exec.Command(osuExe)
		}
		c.Dir = osuDir
		c.Env = os.Environ()
		return c, source, nil
	}

	if osuWine, err := findOsuWine(); err == nil {
		var args []string
		switch {
		case inject:
			args = []string{"--wine", patcher, "--server", srv}
		case sameFolder(osuDir, osuWineManagedPath()):
			args = nil // osu-wine already points at this install; let it do the full launch
		default:
			args = []string{"--wine", osuExe}
		}
		c := exec.Command(osuWine, args...)
		c.Dir = osuDir
		c.Env = os.Environ()
		return c, strings.TrimSpace(osuWine + " " + strings.Join(args, " ")), nil
	}

	wine, err := resolveBinary("wine")
	if err != nil {
		return nil, "", fmt.Errorf("neither osu-wine nor wine found in PATH")
	}
	target := osuExe
	var extra []string
	if inject {
		target, extra = patcher, []string{"--server", srv}
	}
	c := exec.Command(wine, append([]string{target}, extra...)...)
	c.Dir = osuDir
	c.Env = os.Environ()
	return c, "wine " + target, nil
}

func findOsuWine() (string, error) {
	candidates := []string{"osu-wine"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", "osu-wine"))
	}
	for _, cand := range candidates {
		if bin, err := resolveBinary(cand); err == nil {
			return bin, nil
		}
	}
	return "", fmt.Errorf("osu-wine not found")
}

func sameFolder(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, err := filepath.Abs(normalizePath(a))
	if err != nil {
		return false
	}
	rb, err := filepath.Abs(normalizePath(b))
	if err != nil {
		return false
	}
	return filepath.Clean(ra) == filepath.Clean(rb)
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// normalizePath strips a file:// prefix and percent-decodes the path so values
// coming back from fyne's file dialog (which sometimes hand back URI-encoded
// strings with %21 for !) work the same as a manually typed path.
//
// Windows quirk: Fyne's folder dialog returns URIs whose .Path() looks like
// "/C:/Users/foo/osu!" — a leading slash before the drive letter. os.Stat on
// that path fails, so the patcher thinks the folder doesn't exist. We strip
// the leading slash whenever the second character is a drive-letter colon.
func normalizePath(p string) string {
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "file://") {
		p = strings.TrimPrefix(p, "file://")
	}
	if dec, err := url.PathUnescape(p); err == nil {
		p = dec
	}
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return p
}

func resolveBinary(name string) (string, error) {
	if strings.ContainsRune(name, '/') {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
		return "", fmt.Errorf("%s not found", name)
	}
	return exec.LookPath(name)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func labeledRow(label string, w fyne.CanvasObject) *fyne.Container {
	lbl := widget.NewLabel(label)
	lbl.Wrapping = fyne.TextTruncate
	return container.NewGridWithColumns(2, lbl, w)
}

func floatEntry(val float64, min, max float64, onChange func(float64)) *widget.Entry {
	e := widget.NewEntry()
	e.SetText(strconv.FormatFloat(val, 'f', 2, 64))
	e.OnChanged = func(s string) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return
		}
		if v < min {
			v = min
		}
		if v > max {
			v = max
		}
		onChange(v)
		buildCanvasObjects(true)
		refreshLayersList()
	}
	return e
}

func float32Entry(val float32, onChange func(float32)) *widget.Entry {
	e := widget.NewEntry()
	e.SetText(fmt.Sprintf("%.0f", val))
	e.OnChanged = func(s string) {
		v, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return
		}
		onChange(float32(v))
		buildCanvasObjects(true)
	}
	return e
}

func uint8Entry(val uint8, onChange func(uint8)) *widget.Entry {
	e := widget.NewEntry()
	e.SetText(fmt.Sprintf("%d", val))
	e.OnChanged = func(s string) {
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			return
		}
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		onChange(uint8(v))
		buildCanvasObjects(true)
	}
	return e
}

func colorRows(label string, r, g, b uint8,
	onR, onG, onB func(uint8)) []fyne.CanvasObject {
	return []fyne.CanvasObject{
		widget.NewLabelWithStyle(label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		labeledRow("  R (0-255)", uint8Entry(r, onR)),
		labeledRow("  G (0-255)", uint8Entry(g, onG)),
		labeledRow("  B (0-255)", uint8Entry(b, onB)),
	}
}

func setInspectorContent(content fyne.CanvasObject) {
	if inspectorPanel == nil {
		return
	}
	inspectorPanel.Objects = []fyne.CanvasObject{content}
	inspectorPanel.Refresh()
}

func showPropertiesPanel(el *UIElement) {
	selectedElement = el
	if el == nil {
		hint := canvas.NewText("select an element on the canvas", colorTextMute)
		hint.TextSize = 12
		hint.Alignment = fyne.TextAlignCenter
		setInspectorContent(container.NewPadded(container.NewCenter(hint)))
		return
	}
	var items []fyne.CanvasObject
	typeTxt := canvas.NewText(strings.ToUpper(el.Type), colorAccent)
	typeTxt.TextSize = 11
	typeTxt.TextStyle = fyne.TextStyle{Bold: true}
	idTxt := canvas.NewText(el.ID, colorTextStrng)
	idTxt.TextSize = 15
	idTxt.TextStyle = fyne.TextStyle{Bold: true}
	items = append(items, container.NewVBox(typeTxt, idTxt), widget.NewSeparator())
	switch el.Type {
	case "background":
		items = append(items, buildBackgroundInspector(el)...)
	case "text":
		items = append(items, buildTextInspector(el)...)
	case "button":
		items = append(items, buildButtonInspector(el)...)
	case "module":
		items = append(items, buildModuleInspector(el)...)
	case "topplays":
		items = append(items, buildTopPlaysInspector(el)...)
	}
	if el.Type != "background" {
		items = append(items, widget.NewSeparator())
		items = append(items, widget.NewButton("🗑 Delete Object", func() {
			deleteElement(el)
		}))
	}
	setInspectorContent(container.NewVBox(items...))
}

func buildBackgroundInspector(el *UIElement) []fyne.CanvasObject {
	imageLabel := widget.NewLabel("—")
	if el.Image != "" {
		imageLabel.SetText(el.Image)
	}
	uploadBtn := widget.NewButton("Upload Image...", func() {
		fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			fileName, err := copyImageToTheme(uc.URI().Path())
			if err != nil {
				notifyError(err)
				return
			}
			el.Image = fileName
			imageLabel.SetText(fileName)
			buildCanvasObjects(true)
		}, mainWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png"}))
		fd.Show()
	})
	out := []fyne.CanvasObject{
		labeledRow("Opacity (0-1)", floatEntry(el.Opacity, 0.0, 1.0, func(v float64) { el.Opacity = v })),
		widget.NewSeparator(),
		uploadBtn,
		container.NewHBox(widget.NewIcon(theme.MediaPhotoIcon()), imageLabel),
		widget.NewSeparator(),
	}
	out = append(out, colorRows("Background Color",
		el.ColorR, el.ColorG, el.ColorB,
		func(v uint8) { el.ColorR = v },
		func(v uint8) { el.ColorG = v },
		func(v uint8) { el.ColorB = v },
	)...)
	return out
}

func buildTextInspector(el *UIElement) []fyne.CanvasObject {
	textEntry := widget.NewMultiLineEntry()
	textEntry.SetText(el.Text)
	textEntry.OnChanged = func(s string) {
		el.Text = s
		buildCanvasObjects(true)
	}
	out := []fyne.CanvasObject{
		labeledRow("X", float32Entry(el.X, func(v float32) { el.X = v })),
		labeledRow("Y", float32Entry(el.Y, func(v float32) { el.Y = v })),
		labeledRow("Opacity (0-1)", floatEntry(el.Opacity, 0, 1, func(v float64) { el.Opacity = v })),
		labeledRow("Font Size", float32Entry(el.FontSize, func(v float32) { el.FontSize = v })),
		widget.NewSeparator(),
		widget.NewLabel("Content:"),
		textEntry,
		widget.NewSeparator(),
	}
	out = append(out, colorRows("Text Color",
		el.TextColorR, el.TextColorG, el.TextColorB,
		func(v uint8) { el.TextColorR = v },
		func(v uint8) { el.TextColorG = v },
		func(v uint8) { el.TextColorB = v },
	)...)
	return out
}

func buildButtonInspector(el *UIElement) []fyne.CanvasObject {
	textEntry := widget.NewEntry()
	textEntry.SetText(el.Text)
	textEntry.OnChanged = func(s string) {
		el.Text = s
		buildCanvasObjects(true)
	}
	actionEntry := widget.NewEntry()
	actionEntry.SetText(el.Action)
	actionEntry.SetPlaceHolder("https://... or internal://launch | internal://switch_server")
	actionEntry.OnChanged = func(s string) { el.Action = s }
	out := []fyne.CanvasObject{
		labeledRow("X", float32Entry(el.X, func(v float32) { el.X = v })),
		labeledRow("Y", float32Entry(el.Y, func(v float32) { el.Y = v })),
		labeledRow("Width", float32Entry(el.Width, func(v float32) { el.Width = v })),
		labeledRow("Height", float32Entry(el.Height, func(v float32) { el.Height = v })),
		labeledRow("Opacity (0-1)", floatEntry(el.Opacity, 0, 1, func(v float64) { el.Opacity = v })),
		labeledRow("Corner Radius", float32Entry(el.CornerRadius, func(v float32) { el.CornerRadius = v })),
		widget.NewSeparator(),
		labeledRow("Label", textEntry),
		labeledRow("Action", actionEntry),
		widget.NewSeparator(),
	}
	out = append(out, colorRows("Button Color",
		el.ColorR, el.ColorG, el.ColorB,
		func(v uint8) { el.ColorR = v },
		func(v uint8) { el.ColorG = v },
		func(v uint8) { el.ColorB = v },
	)...)
	out = append(out, widget.NewSeparator())
	out = append(out, colorRows("Text Color",
		el.TextColorR, el.TextColorG, el.TextColorB,
		func(v uint8) { el.TextColorR = v },
		func(v uint8) { el.TextColorG = v },
		func(v uint8) { el.TextColorB = v },
	)...)
	return out
}

func buildModuleInspector(el *UIElement) []fyne.CanvasObject {
	if el.ModuleMode == "" {
		el.ModuleMode = "info"
	}

	epEntry := widget.NewEntry()
	epEntry.SetText(el.Endpoint)
	epEntry.OnChanged = func(s string) {
		el.Endpoint = s
		buildCanvasObjects(true)
	}

	modeLabel := widget.NewLabel("Display Mode:")

	infoBtn := widget.NewButton("Info", nil)
	webBtn := widget.NewButton("WebView", nil)

	betaWarn := widget.NewLabelWithStyle(
		"⚠ WebView is BETA — renders as a static screenshot.\nWayland support is unstable; click the tile to open in a real browser.",
		fyne.TextAlignLeading,
		fyne.TextStyle{Italic: true},
	)
	betaWarn.Wrapping = fyne.TextWrapWord

	updateModeButtons := func() {
		if el.ModuleMode == "url" {
			infoBtn.Importance = widget.LowImportance
			webBtn.Importance = widget.HighImportance
			betaWarn.Show()
		} else {
			infoBtn.Importance = widget.HighImportance
			webBtn.Importance = widget.LowImportance
			betaWarn.Hide()
		}
		infoBtn.Refresh()
		webBtn.Refresh()
	}

	infoBtn.OnTapped = func() {
		el.ModuleMode = "info"
		updateModeButtons()
		buildCanvasObjects(true)
	}
	webBtn.OnTapped = func() {
		el.ModuleMode = "url"
		updateModeButtons()
		buildCanvasObjects(true)
	}

	updateModeButtons()

	modeToggle := container.NewGridWithColumns(2, infoBtn, webBtn)

	out := []fyne.CanvasObject{
		labeledRow("X", float32Entry(el.X, func(v float32) { el.X = v })),
		labeledRow("Y", float32Entry(el.Y, func(v float32) { el.Y = v })),
		labeledRow("Width", float32Entry(el.Width, func(v float32) { el.Width = v })),
		labeledRow("Height", float32Entry(el.Height, func(v float32) { el.Height = v })),
		labeledRow("Opacity (0-1)", floatEntry(el.Opacity, 0, 1, func(v float64) { el.Opacity = v })),
		labeledRow("Corner Radius", float32Entry(el.CornerRadius, func(v float32) { el.CornerRadius = v })),
		widget.NewSeparator(),
		labeledRow("Endpoint", epEntry),
		widget.NewSeparator(),
		modeLabel,
		modeToggle,
		betaWarn,
		widget.NewSeparator(),
	}

	out = append(out, colorRows("Module Color",
		el.ColorR, el.ColorG, el.ColorB,
		func(v uint8) { el.ColorR = v },
		func(v uint8) { el.ColorG = v },
		func(v uint8) { el.ColorB = v },
	)...)
	out = append(out, widget.NewSeparator())
	out = append(out, colorRows("Text Color",
		el.TextColorR, el.TextColorG, el.TextColorB,
		func(v uint8) { el.TextColorR = v },
		func(v uint8) { el.TextColorG = v },
		func(v uint8) { el.TextColorB = v },
	)...)
	return out
}

func buildTopPlaysInspector(el *UIElement) []fyne.CanvasObject {
	epEntry := widget.NewEntry()
	epEntry.SetText(el.Endpoint)
	epEntry.SetPlaceHolder("https://.../api/v1/users/scores/best?id=…&l=5")
	epEntry.OnChanged = func(s string) { el.Endpoint = s }

	clearBtn := widget.NewButton("Disconnect", func() {
		el.Endpoint = ""
		epEntry.SetText("")
		saveConfig()
		buildCanvasObjects(isEditorMode)
	})
	clearBtn.Importance = widget.DangerImportance

	out := []fyne.CanvasObject{
		labeledRow("X", float32Entry(el.X, func(v float32) { el.X = v })),
		labeledRow("Y", float32Entry(el.Y, func(v float32) { el.Y = v })),
		labeledRow("Width", float32Entry(el.Width, func(v float32) { el.Width = v })),
		labeledRow("Height", float32Entry(el.Height, func(v float32) { el.Height = v })),
		labeledRow("Opacity (0-1)", floatEntry(el.Opacity, 0, 1, func(v float64) { el.Opacity = v })),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("API Endpoint", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		epEntry,
		clearBtn,
		widget.NewSeparator(),
	}
	out = append(out, colorRows("Placeholder Color",
		el.ColorR, el.ColorG, el.ColorB,
		func(v uint8) { el.ColorR = v },
		func(v uint8) { el.ColorG = v },
		func(v uint8) { el.ColorB = v },
	)...)
	out = append(out, widget.NewSeparator())
	out = append(out, colorRows("Text Color",
		el.TextColorR, el.TextColorG, el.TextColorB,
		func(v uint8) { el.TextColorR = v },
		func(v uint8) { el.TextColorG = v },
		func(v uint8) { el.TextColorB = v },
	)...)
	return out
}

func deleteElement(el *UIElement) {
	confirmModal(T("confirm_delete"), fmt.Sprintf("Delete element '%s'? This cannot be undone.", el.ID), "Delete", func() {
		newEl := make([]*UIElement, 0, len(activeTheme.Elements))
		for _, e := range activeTheme.Elements {
			if e != el {
				newEl = append(newEl, e)
			}
		}
		activeTheme.Elements = newEl
		selectedElement = nil
		showPropertiesPanel(nil)
		buildCanvasObjects(true)
		refreshLayersList()
	})
}

func addElement(el *UIElement) {
	activeTheme.Elements = append(activeTheme.Elements, el)
	buildCanvasObjects(true)
	refreshLayersList()
	showPropertiesPanel(el)
}

func nextID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, len(activeTheme.Elements))
}

func showTemplatesPanel() {
	type tmplDef struct {
		Name    string
		Builder func() *UIElement
	}
	templates := []tmplDef{
		{"Text", func() *UIElement {
			return &UIElement{
				ID: nextID("txt"), Type: "text",
				Text: "New Text", X: 50, Y: 50,
				FontSize: 20, Opacity: 1.0,
				TextColorR: 255, TextColorG: 255, TextColorB: 255,
			}
		}},
		{"Button", func() *UIElement {
			return &UIElement{
				ID: nextID("btn"), Type: "button",
				Text: "Click Me", X: 50, Y: 100,
				Width: 150, Height: 48, Opacity: 1.0,
				ColorR: 255, ColorG: 102, ColorB: 153,
				TextColorR: 255, TextColorG: 255, TextColorB: 255,
				Action: "internal://launch",
			}
		}},
		{"API Module", func() *UIElement {
			return &UIElement{
				ID:         nextID("mod"),
				Type:       "module",
				Endpoint:   "https://example.com",
				ModuleMode: "info",
				X:          50, Y: 200, Width: 220, Height: 100, Opacity: 1.0,
				ColorR: 50, ColorG: 50, ColorB: 70,
				TextColorR: 180, TextColorG: 180, TextColorB: 255,
			}
		}},
		{"Switch Server", func() *UIElement {
			return &UIElement{
				ID: nextID("btn_srv"), Type: "button",
				Text: "🌐  Switch Server", X: 60, Y: 350,
				Width: 220, Height: 44, Opacity: 1.0,
				ColorR: 60, ColorG: 80, ColorB: 140,
				TextColorR: 255, TextColorG: 255, TextColorB: 255,
				Action: "internal://switch_server",
			}
		}},
		{"Top Plays", func() *UIElement {
			return &UIElement{
				ID:       nextID("topplays"),
				Type:     "topplays",
				Endpoint: "", // user fills via connect screen
				X:        50, Y: 130, Width: 440, Height: 400, Opacity: 1.0,
				ColorR: 25, ColorG: 20, ColorB: 40,
				TextColorR: 255, TextColorG: 100, TextColorB: 165,
			}
		}},
	}
	header := canvas.NewText("ADD ELEMENT", colorAccent)
	header.TextSize = 11
	header.TextStyle = fyne.TextStyle{Bold: true}

	var btns []fyne.CanvasObject
	btns = append(btns, header, widget.NewSeparator())
	for _, t := range templates {
		t := t
		b := widget.NewButtonWithIcon("  "+t.Name, theme.ContentAddIcon(), func() { addElement(t.Builder()) })
		b.Alignment = widget.ButtonAlignLeading
		btns = append(btns, b)
	}
	setInspectorContent(container.NewVBox(btns...))
}

func refreshLayersList() {
	if layersList != nil {
		layersList.Refresh()
	}
}

func buildLayersPanel() fyne.CanvasObject {
	layersList = widget.NewList(
		func() int { return len(activeTheme.Elements) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewIcon(theme.DocumentIcon()), widget.NewLabel(""))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c := o.(*fyne.Container)
			el := activeTheme.Elements[i]
			icon := c.Objects[0].(*widget.Icon)
			label := c.Objects[1].(*widget.Label)
			switch el.Type {
			case "background":
				icon.SetResource(theme.MediaPhotoIcon())
			case "button":
				icon.SetResource(theme.ConfirmIcon())
			case "module":
				icon.SetResource(theme.ComputerIcon())
			case "topplays":
				icon.SetResource(theme.ListIcon())
			default:
				icon.SetResource(theme.DocumentIcon())
			}
			label.SetText(fmt.Sprintf("%s · %s", el.Type, el.ID))
		},
	)
	layersList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(activeTheme.Elements) {
			showPropertiesPanel(activeTheme.Elements[id])
		}
	}
	header := canvas.NewText("LAYERS", colorAccent)
	header.TextSize = 11
	header.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		nil, nil, nil, layersList,
	)
}

func showFileMenu() {
	content := container.NewVBox(
		widget.NewLabelWithStyle(T("menu_file"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewButton(T("new_theme"), func() {
			confirmModal(T("new_theme"), T("create_confirm"), "Create new", func() {
				activeTheme = ThemeConfig{Name: "New Theme", Elements: defaultElements()}
				switchToEditor()
			})
		}),
		widget.NewButton("Open Config...", func() {
			fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
				if err != nil || uc == nil {
					return
				}
				defer uc.Close()
				data, err := io.ReadAll(uc)
				if err != nil {
					notifyError(err)
					return
				}
				var cfg ThemeConfig
				if err := json.Unmarshal(data, &cfg); err != nil {
					notifyError(err)
					return
				}
				activeTheme = cfg
				switchToEditor()
			}, mainWindow)
			fd.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
			fd.Show()
		}),
		widget.NewButton("Save as New Theme...", promptSaveAsNewTheme),
		widget.NewButton("Export .patchercfg", exportToPatcherCfg),
		widget.NewSeparator(),
		widget.NewButton(T("back"), switchToEditor),
	)

	bg := canvas.NewRectangle(colorSettings)
	mainWindow.SetContent(container.NewStack(
		bg,
		container.NewCenter(container.NewVBox(content)),
	))
}

func showSettings() {
	buildSettingsScreen()
}

type settingsPage struct {
	Key      string
	Title    string
	Subtitle string
	Icon     fyne.Resource
	Build    func() fyne.CanvasObject
}

var activeSettingsPage = "appearance"

func buildSettingsScreen() {
	pages := []settingsPage{
		{"appearance", T("page_appearance"), T("page_appearance_sub"), theme.ColorPaletteIcon(), buildAppearancePage},
		{"themes", T("page_themes"), T("page_themes_sub"), theme.GridIcon(), buildThemesPage},
		{"account", T("page_account"), T("page_account_sub"), theme.AccountIcon(), buildAccountPage},
		{"game", T("page_game"), T("page_game_sub"), theme.ComputerIcon(), buildGamePage},
		{"credits", T("page_credits"), T("page_credits_sub"), theme.InfoIcon(), buildCreditsPage},
	}

	pageByKey := map[string]settingsPage{}
	for _, p := range pages {
		pageByKey[p.Key] = p
	}
	if _, ok := pageByKey[activeSettingsPage]; !ok {
		activeSettingsPage = pages[0].Key
	}

	// ── content area ──
	contentHolder := container.NewMax()
	navButtons := map[string]*widget.Button{}

	renderPage := func(key string) {
		p, ok := pageByKey[key]
		if !ok {
			return
		}
		activeSettingsPage = key
		card := pageCard(p.Title, p.Subtitle, container.NewVScroll(p.Build()))
		contentHolder.Objects = []fyne.CanvasObject{card}
		contentHolder.Refresh()

		for k, b := range navButtons {
			if k == key {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.LowImportance
			}
			b.Refresh()
		}
	}

	// ── nav buttons ──
	navItems := make([]fyne.CanvasObject, 0, len(pages)+1)
	navItems = append(navItems,
		canvas.NewText(strings.ToUpper(T("settings_title")), colorAccent),
		widget.NewSeparator(),
	)
	for _, p := range pages {
		p := p
		btn := widget.NewButtonWithIcon("  "+p.Title, p.Icon, func() { renderPage(p.Key) })
		btn.Alignment = widget.ButtonAlignLeading
		if p.Key == activeSettingsPage {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.LowImportance
		}
		navButtons[p.Key] = btn
		navItems = append(navItems, btn)
	}

	sidebarBg := canvas.NewRectangle(colorSidebar)
	sidebar := container.NewStack(sidebarBg, container.NewPadded(container.NewVBox(navItems...)))
	sidebarWrap := container.New(&fixedWidthLayout{width: 220}, sidebar)

	renderPage(activeSettingsPage)

	// ── top bar ──
	backBtn := widget.NewButtonWithIcon(T("back"), theme.NavigateBackIcon(), switchToLauncher)
	titleLabel := canvas.NewText(T("settings_title"), colorTextStrng)
	titleLabel.TextSize = 16
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Alignment = fyne.TextAlignCenter

	topBar := container.NewBorder(nil, nil, backBtn, nil, container.NewCenter(titleLabel))
	topBarBg := canvas.NewRectangle(colorTopBar)
	topBarFull := container.NewStack(topBarBg, container.NewPadded(topBar))

	bg := canvas.NewRectangle(colorSettings)
	body := container.NewBorder(nil, nil, sidebarWrap, nil, container.NewPadded(contentHolder))
	mainWindow.SetContent(container.NewStack(
		bg,
		container.NewBorder(topBarFull, nil, nil, nil, body),
	))
}

// ── page builders ────────────────────────────────────────────────────────────

func buildAppearancePage() fyne.CanvasObject {
	langSelect := widget.NewSelect(availableLanguages(), nil)
	langSelect.SetSelected(appSettings.Language)
	langSelect.OnChanged = func(s string) {
		if s == appSettings.Language {
			return
		}
		appSettings.Language = s
		saveAppSettings()
		buildSettingsScreen()
	}

	// Display name → stamped as the "author" on any theme you save & share.
	userEntry := widget.NewEntry()
	userEntry.SetText(appSettings.UserName)
	userEntry.SetPlaceHolder("e.g. tazik — shown as the theme author when you share one")
	// Auto-saved like every other field on these pages — a lone Save button here
	// was the one place you could lose an edit by navigating away.
	userEntry.OnChanged = func(s string) {
		appSettings.UserName = s
		saveAppSettings()
	}

	openEditorBtn := widget.NewButtonWithIcon(T("open_editor"), theme.DocumentCreateIcon(), switchToEditor)
	openEditorBtn.Importance = widget.HighImportance

	langCard := subCard(container.NewVBox(
		fieldLabel(strings.ToUpper(T("lang_label"))),
		langSelect,
	))

	userCard := subCard(container.NewVBox(
		fieldLabel(strings.ToUpper(T("display_name"))),
		widget.NewLabel(T("display_name_hint")),
		userEntry,
	))

	editorCard := subCard(container.NewVBox(
		fieldLabel(strings.ToUpper(T("ui_editor"))),
		widget.NewLabel(T("ui_editor_hint")),
		container.NewHBox(openEditorBtn),
	))

	return container.NewVBox(langCard, userCard, editorCard)
}

func buildAccountPage() fyne.CanvasObject {
	tpEntry := widget.NewEntry()
	tpEntry.SetText(appSettings.TopPlaysURL)
	tpEntry.SetPlaceHolder("https://…/api/v1/users/scores/best?id=…&mode=0&rx=0&l=5")
	tpEntry.OnChanged = func(s string) { appSettings.TopPlaysURL = s }

	tpUserIDEntry := widget.NewEntry()
	tpUserIDEntry.SetPlaceHolder("your numeric user id (e.g. 1234)")

	presetNames := make([]string, 0, len(topPlaysPresets))
	for _, p := range topPlaysPresets {
		presetNames = append(presetNames, p.Name)
	}
	tpPresetSelect := widget.NewSelect(presetNames, nil)
	tpPresetSelect.PlaceHolder = "choose server…"
	tpPresetSelect.OnChanged = func(name string) {
		id := strings.TrimSpace(tpUserIDEntry.Text)
		if id == "" {
			id = "USER_ID"
		}
		for _, p := range topPlaysPresets {
			if p.Name == name {
				tpEntry.SetText(fmt.Sprintf(p.URLTemplate, id))
				appSettings.TopPlaysURL = tpEntry.Text
				return
			}
		}
	}
	tpUserIDEntry.OnChanged = func(s string) {
		if tpPresetSelect.Selected != "" {
			tpPresetSelect.OnChanged(tpPresetSelect.Selected)
		}
	}

	tpStatusLbl := widget.NewLabel("")

	tpSaveBtn := widget.NewButtonWithIcon("Save & Test", theme.ConfirmIcon(), func() {
		urlStr := strings.TrimSpace(tpEntry.Text)
		tpStatusLbl.SetText("checking…")
		go func() {
			_, err := fetchTopPlays(urlStr)
			if err != nil {
				tpStatusLbl.SetText("⚠ " + err.Error())
				return
			}
			appSettings.TopPlaysURL = urlStr
			saveAppSettings()
			for _, el := range activeTheme.Elements {
				if el.Type == "topplays" && el.Endpoint == "" {
					el.Endpoint = urlStr
				}
			}
			saveConfig()
			tpStatusLbl.SetText("✓ connected")
		}()
	})
	tpSaveBtn.Importance = widget.HighImportance

	tpClearBtn := widget.NewButtonWithIcon("Disconnect", theme.CancelIcon(), func() {
		appSettings.TopPlaysURL = ""
		tpEntry.SetText("")
		tpStatusLbl.SetText("")
		saveAppSettings()
	})

	quickCard := subCard(container.NewVBox(
		fieldLabel("QUICK SETUP"),
		widget.NewLabel("Pick a server, then drop in your user id. The URL is generated for you."),
		labeledRow("Server", tpPresetSelect),
		labeledRow("User ID", tpUserIDEntry),
	))

	urlCard := subCard(container.NewVBox(
		fieldLabel("API URL"),
		tpEntry,
		widget.NewLabel("Example: …/api/v1/users/scores/best?id=1234&mode=0&rx=0&l=5"),
		container.NewHBox(tpSaveBtn, tpClearBtn),
		tpStatusLbl,
	))

	return container.NewVBox(quickCard, urlCard)
}

func buildGamePage() fyne.CanvasObject {
	// ── osu! folder ──
	statusTxt := canvas.NewText("", colorTextMute)
	statusTxt.TextSize = 11
	statusTxt.TextStyle = fyne.TextStyle{Italic: true}

	osuFolderEntry := widget.NewEntry()
	osuFolderEntry.SetPlaceHolder(T("osu_folder_ph"))
	osuFolderEntry.SetText(appSettings.OsuFolder)

	refreshFolderStatus := func() {
		folder := normalizePath(strings.TrimSpace(appSettings.OsuFolder))
		switch {
		case folder == "":
			statusTxt.Text = T("folder_none")
			statusTxt.Color = colorTextMute
		case !dirExists(folder):
			statusTxt.Text = T("folder_missing")
			statusTxt.Color = colorDanger
		case !fileExists(filepath.Join(folder, osuExeName)):
			statusTxt.Text = T("folder_no_exe")
			statusTxt.Color = colorDanger
		default:
			statusTxt.Text = T("folder_ok")
			statusTxt.Color = colorOk
		}
		statusTxt.Refresh()
	}
	refreshFolderStatus()

	setFolder := func(path string) {
		appSettings.OsuFolder = path
		osuFolderEntry.SetText(path)
		saveAppSettings()
		refreshFolderStatus()
	}

	// Auto-save on every keystroke so nobody can land in the "typed the path,
	// forgot to hit Save, Launch fails" trap. The raw text is kept as-is and
	// normalised on read instead (file://, URI escapes, the Windows /C:/ quirk).
	osuFolderEntry.OnChanged = func(s string) {
		appSettings.OsuFolder = s
		saveAppSettings()
		refreshFolderStatus()
	}

	browseBtn := widget.NewButtonWithIcon(T("browse_btn"), theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			setFolder(normalizePath(uri.Path()))
		}, mainWindow)
	})
	detectBtn := widget.NewButtonWithIcon(T("autodetect_btn"), theme.SearchIcon(), func() {
		found := detectOsuFolder()
		if found == "" {
			toast(toastOpts{level: modalWarn, message: T("autodetect_fail")})
			return
		}
		setFolder(found)
		toast(toastOpts{level: modalSuccess, message: "found " + found})
	})

	folderCard := subCard(container.NewVBox(
		fieldLabel(T("osu_folder_label")),
		widget.NewLabel(T("osu_folder_hint")),
		osuFolderEntry,
		container.NewHBox(browseBtn, detectBtn),
		statusTxt,
	))

	// ── server ──
	customServerLabel := T("server_custom")

	serverNames := make([]string, 0, len(serverPresets)+1)
	for _, p := range serverPresets {
		serverNames = append(serverNames, p.Name)
	}
	serverNames = append(serverNames, customServerLabel)

	modeTxt := canvas.NewText("", colorTextMute)
	modeTxt.TextSize = 11
	refreshMode := func() {
		if needsPatcher(currentServer()) {
			modeTxt.Text = T("mode_patched")
			modeTxt.Color = colorAccent
		} else {
			modeTxt.Text = T("mode_vanilla")
			modeTxt.Color = colorTextMute
		}
		modeTxt.Refresh()
	}

	customServerEntry := widget.NewEntry()
	customServerEntry.SetPlaceHolder("mysrv.example.com")

	selectedName := customServerLabel
	for _, p := range serverPresets {
		if p.DevServer == currentServer() {
			selectedName = p.Name
			break
		}
	}
	if selectedName == customServerLabel {
		customServerEntry.SetText(currentServer())
	} else {
		customServerEntry.Hide()
	}

	serverSelect := widget.NewSelect(serverNames, nil)
	serverSelect.SetSelected(selectedName)
	serverSelect.OnChanged = func(name string) {
		if name == customServerLabel {
			customServerEntry.Show()
			if v := strings.TrimSpace(customServerEntry.Text); v != "" {
				appSettings.Server = v
			}
		} else {
			customServerEntry.Hide()
			for _, p := range serverPresets {
				if p.Name == name {
					appSettings.Server = p.DevServer
					break
				}
			}
		}
		saveAppSettings()
		refreshMode()
	}
	customServerEntry.OnChanged = func(s string) {
		if serverSelect.Selected != customServerLabel {
			return
		}
		appSettings.Server = strings.TrimSpace(s)
		saveAppSettings()
		refreshMode()
	}
	refreshMode()

	launchBtn := widget.NewButtonWithIcon(T("launch_btn"), theme.MediaPlayIcon(), launchOsu)
	launchBtn.Importance = widget.HighImportance

	serverCard := subCard(container.NewVBox(
		fieldLabel(T("server_label")),
		serverSelect,
		customServerEntry,
		modeTxt,
		widget.NewSeparator(),
		container.NewHBox(launchBtn),
	))

	return container.NewVBox(folderCard, serverCard, buildAdvancedSection())
}

// buildAdvancedSection holds the escape hatches: a launch command that replaces
// the pipeline outright, and a patcher exe for builds without a bundled one.
// Collapsed by default — nobody needs these to play.
func buildAdvancedSection() fyne.CanvasObject {
	launchCmdEntry := widget.NewEntry()
	launchCmdEntry.SetText(appSettings.LaunchCommand)
	if runtime.GOOS == "windows" {
		launchCmdEntry.SetPlaceHolder("leave empty to launch osu! normally")
	} else {
		launchCmdEntry.SetPlaceHolder("leave empty for auto (osu-wine → wine)")
	}
	launchCmdEntry.OnChanged = func(s string) {
		appSettings.LaunchCommand = s
		saveAppSettings()
	}

	examples := canvas.NewText("Examples:  osu-wine  |  lutris lutris:rungame/osu-stable", colorTextMute)
	examples.TextSize = 10

	patcherPathEntry := widget.NewEntry()
	patcherPathEntry.SetText(appSettings.PatcherPath)
	patcherPathEntry.SetPlaceHolder("path to an externally built " + patcherFilename)
	patcherPathEntry.OnChanged = func(s string) {
		appSettings.PatcherPath = s
		saveAppSettings()
	}

	patcherBrowseBtn := widget.NewButtonWithIcon(T("browse_btn"), theme.FolderOpenIcon(), func() {
		fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			appSettings.PatcherPath = normalizePath(uc.URI().Path())
			patcherPathEntry.SetText(appSettings.PatcherPath)
			saveAppSettings()
		}, mainWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".exe"}))
		fd.Show()
	})
	patcherClearBtn := widget.NewButtonWithIcon(T("clear_btn"), theme.ContentClearIcon(), func() {
		appSettings.PatcherPath = ""
		patcherPathEntry.SetText("")
		saveAppSettings()
	})

	bundleStatus := canvas.NewText("bundled patcher: ✓ ready", colorOk)
	if !hasEmbeddedPatcher() {
		bundleStatus.Text = "bundled patcher: ✕ missing — rebuild via build.sh"
		bundleStatus.Color = colorDanger
	}
	bundleStatus.TextSize = 11
	bundleStatus.TextStyle = fyne.TextStyle{Italic: true}

	body := container.NewVBox(
		fieldLabel(T("launch_cmd_label")),
		widget.NewLabel(T("launch_cmd_hint")),
		launchCmdEntry,
		examples,
		widget.NewSeparator(),
		fieldLabel(T("patcher_exe_label")),
		widget.NewLabel(T("patcher_exe_hint")),
		patcherPathEntry,
		container.NewHBox(patcherBrowseBtn, patcherClearBtn),
		bundleStatus,
	)

	acc := widget.NewAccordion(widget.NewAccordionItem(T("advanced"), body))
	return acc
}

func buildCreditsPage() fyne.CanvasObject {
	person := func(name, role, ghURL string) fyne.CanvasObject {
		nameTxt := canvas.NewText(name, colorAccent)
		nameTxt.TextSize = 18
		nameTxt.TextStyle = fyne.TextStyle{Bold: true}
		roleTxt := canvas.NewText(role, colorTextMute)
		roleTxt.TextSize = 11
		return subCard(container.NewVBox(
			nameTxt,
			roleTxt,
			makeLinkButton("github.com/"+strings.TrimPrefix(ghURL, "https://github.com/"), ghURL),
		))
	}
	return container.NewVBox(
		person("taziksfear", "Main Developer", "https://github.com/taziksfear"),
		person("SimplyAe", "Helped with the UI and the patcher bridge", "https://github.com/SimplyAe"),
	)
}

func sectionTitle(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// pageCard wraps a settings section in a dark rounded panel with a pink accent
// bar and a big title + subtitle. Used to make each settings page look like a
// distinct, modern card.
func pageCard(title, subtitle string, body fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorPanel)
	bg.CornerRadius = 14

	accent := canvas.NewRectangle(colorAccent)
	accent.SetMinSize(fyne.NewSize(4, 32))

	titleTxt := canvas.NewText(title, colorTextStrng)
	titleTxt.TextSize = 22
	titleTxt.TextStyle = fyne.TextStyle{Bold: true}

	subTxt := canvas.NewText(subtitle, colorTextMute)
	subTxt.TextSize = 11

	titleBlock := container.NewVBox(titleTxt, subTxt)
	header := container.NewBorder(nil, nil, container.NewHBox(accent), nil, titleBlock)

	inner := container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		nil, nil, nil,
		body,
	)
	return container.NewStack(bg, container.NewPadded(inner))
}

// subCard makes a smaller inset panel for grouping related fields inside a page.
func subCard(body fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorPanelHi)
	bg.CornerRadius = 10
	return container.NewStack(bg, container.NewPadded(body))
}

// fieldLabel renders a small uppercase-ish label above an input for that
// "settings page" feel.
func fieldLabel(text string) *canvas.Text {
	t := canvas.NewText(text, colorTextMute)
	t.TextSize = 11
	t.TextStyle = fyne.TextStyle{Bold: true}
	return t
}

// ── styled modal / toast system ─────────────────────────────────────────────

type modalLevel int

const (
	modalInfo modalLevel = iota
	modalSuccess
	modalError
	modalWarn
)

func (l modalLevel) accent() color.NRGBA {
	switch l {
	case modalSuccess:
		return colorOk
	case modalError:
		return colorDanger
	case modalWarn:
		return colorWarn
	default:
		return colorAccent
	}
}

func (l modalLevel) icon() string {
	switch l {
	case modalSuccess:
		return "✓"
	case modalError:
		return "✕"
	case modalWarn:
		return "⚠"
	default:
		return "ℹ"
	}
}

type modalAction struct {
	Label   string
	Primary bool
	Danger  bool
	OnClick func()
}

// showModal pops a centered, styled modal with title + message + custom buttons.
// If actions is empty, an "OK" button is added that just dismisses the modal.
func showModal(level modalLevel, title, message string, actions ...modalAction) {
	var pop *widget.PopUp

	bg := canvas.NewRectangle(colorPanel)
	bg.CornerRadius = 14
	bg.SetMinSize(fyne.NewSize(420, 0))

	accentBar := canvas.NewRectangle(level.accent())
	accentBar.SetMinSize(fyne.NewSize(4, 36))

	iconTxt := canvas.NewText(level.icon(), level.accent())
	iconTxt.TextSize = 22
	iconTxt.TextStyle = fyne.TextStyle{Bold: true}

	titleTxt := canvas.NewText(title, colorTextStrng)
	titleTxt.TextSize = 17
	titleTxt.TextStyle = fyne.TextStyle{Bold: true}

	header := container.NewBorder(nil, nil,
		container.NewHBox(accentBar, iconTxt),
		nil,
		container.NewCenter(titleTxt),
	)

	msgLbl := widget.NewLabel(message)
	msgLbl.Wrapping = fyne.TextWrapWord

	if len(actions) == 0 {
		actions = []modalAction{{Label: "OK", Primary: true}}
	}
	btnRow := container.NewHBox(layout.NewSpacer())
	for _, a := range actions {
		a := a
		b := widget.NewButton(a.Label, func() {
			if pop != nil {
				pop.Hide()
			}
			if a.OnClick != nil {
				a.OnClick()
			}
		})
		switch {
		case a.Danger:
			b.Importance = widget.DangerImportance
		case a.Primary:
			b.Importance = widget.HighImportance
		}
		btnRow.Add(b)
	}

	body := container.NewBorder(
		container.NewVBox(container.NewPadded(header), widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewPadded(btnRow)),
		nil, nil,
		container.NewPadded(msgLbl),
	)

	card := container.NewStack(bg, body)
	pop = widget.NewModalPopUp(card, mainWindow.Canvas())
	pop.Show()
}

func notify(title, message string) { showModal(modalInfo, title, message) }

// notifyError opens a styled error modal with the error message.
func notifyError(err error) { showModal(modalError, "Error", err.Error()) }

// confirmModal shows a yes/no styled confirmation modal.
func confirmModal(title, message, confirmLabel string, onConfirm func()) {
	showModal(modalWarn, title, message,
		modalAction{Label: "Cancel"},
		modalAction{Label: confirmLabel, Danger: true, OnClick: onConfirm},
	)
}

// toast slides a small non-blocking pill at the bottom-right of the canvas.
// Auto-dismisses after ~2.5s. Use it for "saved", "copied", quick confirmations.
type toastOpts struct {
	level   modalLevel
	message string
}

func toast(opts toastOpts) {
	bg := canvas.NewRectangle(colorPanelHi)
	bg.CornerRadius = 12

	accent := canvas.NewRectangle(opts.level.accent())
	accent.SetMinSize(fyne.NewSize(3, 24))

	iconTxt := canvas.NewText(opts.level.icon(), opts.level.accent())
	iconTxt.TextSize = 14
	iconTxt.TextStyle = fyne.TextStyle{Bold: true}

	msg := canvas.NewText(opts.message, colorTextStrng)
	msg.TextSize = 12

	row := container.NewHBox(accent, iconTxt, msg)
	content := container.NewStack(bg, container.NewPadded(row))

	pop := widget.NewPopUp(content, mainWindow.Canvas())
	sz := mainWindow.Canvas().Size()
	min := content.MinSize()
	pop.ShowAtPosition(fyne.NewPos(sz.Width-min.Width-24, sz.Height-min.Height-24))

	go func() {
		time.Sleep(2500 * time.Millisecond)
		pop.Hide()
	}()
}

func toastSaved(what string) { toast(toastOpts{level: modalSuccess, message: what + " saved"}) }
func toastInfo(msg string)   { toast(toastOpts{level: modalInfo, message: msg}) }

func makeLinkButton(label, rawURL string) *widget.Button {
	return widget.NewButton("🔗 "+label, func() {
		u, err := url.Parse(rawURL)
		if err != nil {
			return
		}
		fyne.CurrentApp().OpenURL(u)
	})
}

func switchToEditor() {
	isEditorMode = true
	selectedElement = nil
	placeholder := canvas.NewText("select an element  ·  or hit Templates", colorTextMute)
	placeholder.TextSize = 12
	placeholder.Alignment = fyne.TextAlignCenter
	inspectorPanel = container.NewVBox(container.NewPadded(container.NewCenter(placeholder)))

	themeName := widget.NewLabelWithStyle(
		activeTheme.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true, Italic: true},
	)

	topBarContent := container.NewBorder(
		nil, nil,
		container.NewHBox(
			widget.NewButton(T("menu_file"), showFileMenu),
			widget.NewButton(T("menu_save"), saveConfig),
		),
		container.NewHBox(
			widget.NewButtonWithIcon(T("menu_templ"), theme.ContentAddIcon(), showTemplatesPanel),
			widget.NewButtonWithIcon(T("menu_exit"), theme.CancelIcon(), switchToLauncher),
		),
		themeName,
	)

	topBarBg := canvas.NewRectangle(colorTopBar)
	topBar := container.NewStack(topBarBg, container.NewPadded(topBarContent))

	inspectorScroll := container.NewVScroll(inspectorPanel)
	layersPanel := buildLayersPanel()

	splitContent := container.NewVSplit(inspectorScroll, layersPanel)
	splitContent.SetOffset(0.5)

	sidebarBg := canvas.NewRectangle(colorSidebar)
	rightSidebar := container.NewStack(sidebarBg, container.NewPadded(splitContent))
	sidebarWrapper := container.New(&fixedWidthLayout{width: 280}, rightSidebar)

	buildCanvasObjects(true)

	editorOverlay := container.NewBorder(topBar, nil, nil, sidebarWrapper, nil)
	mainWindow.SetContent(container.NewStack(liveCanvas, editorOverlay))
}

func switchToLauncher() {
	isEditorMode = false
	selectedElement = nil
	buildCanvasObjects(false)

	settingsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), showSettings)
	topRight := container.NewBorder(
		nil, nil, nil,
		container.NewVBox(settingsBtn),
		layout.NewSpacer(),
	)

	mainWindow.SetContent(container.NewStack(liveCanvas, topRight))
}

type fixedWidthLayout struct{ width float32 }

func (f *fixedWidthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(size.Width-f.width, 0))
		o.Resize(fyne.NewSize(f.width, size.Height))
	}
}

func (f *fixedWidthLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.width, 0)
}

func defaultElements() []*UIElement {
	return []*UIElement{
		{
			ID: "bg_main", Type: "background", Opacity: 1.0,
			ColorR: 15, ColorG: 15, ColorB: 20,
		},
		{
			ID: "title_main", Type: "text",
			Text: "osu! patcher", X: 60, Y: 40,
			FontSize: 32, Opacity: 1.0,
			TextColorR: 255, TextColorG: 102, TextColorB: 153,
		},
		{
			ID: "subtitle", Type: "text",
			Text: "by taziksfear & SimplyAe", X: 60, Y: 85,
			FontSize: 14, Opacity: 0.7,
			TextColorR: 200, TextColorG: 200, TextColorB: 200,
		},
		{
			ID: "btn_play", Type: "button",
			Text: "▶  LAUNCH OSU!", X: 60, Y: 200,
			Width: 220, Height: 56, Opacity: 1.0,
			ColorR: 255, ColorG: 102, ColorB: 153,
			TextColorR: 255, TextColorG: 255, TextColorB: 255,
			Action: "internal://launch",
		},
		{
			ID: "btn_update", Type: "button",
			Text: "⟳  Check Updates", X: 60, Y: 270,
			Width: 220, Height: 44, Opacity: 1.0,
			ColorR: 40, ColorG: 40, ColorB: 55,
			TextColorR: 200, TextColorG: 200, TextColorB: 255,
			Action: "https://github.com/taziksfear",
		},
		{
			ID: "version_label", Type: "text",
			Text: "v1.0.0", X: 60, Y: 580,
			FontSize: 12, Opacity: 0.4,
			TextColorR: 180, TextColorG: 180, TextColorB: 180,
		},
	}
}

func initDefaultTheme() {
	if _, err := os.Stat(configPath()); err == nil {
		if err := loadConfig(); err == nil {
			return
		}
	}
	activeTheme = ThemeConfig{Name: "Default", Elements: defaultElements()}
	ensureThemeDir()
	data, _ := json.MarshalIndent(activeTheme, "", "  ")
	os.WriteFile(configPath(), data, 0644)
}

func openWebView(endpoint, title string, size fyne.Size) {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	w := int(size.Width)
	h := int(size.Height)
	if w < 600 {
		w = 900
	}
	if h < 400 {
		h = 600
	}

	OpenEmbeddedWebView(endpoint, w, h)
}

// patcherTheme keeps Fyne's dark theme but swaps its blue highlight for the
// launcher's pink accent, so primary buttons and the selected settings tab stop
// clashing with everything else on screen.
type patcherTheme struct{ fyne.Theme }

func (t patcherTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return colorAccent
	case theme.ColorNameSelection:
		return color.NRGBA{R: colorAccent.R, G: colorAccent.G, B: colorAccent.B, A: 90}
	}
	return t.Theme.Color(name, variant)
}

func main() {
	currentApp = app.New()
	currentApp.Settings().SetTheme(patcherTheme{Theme: theme.DarkTheme()})

	mainWindow = currentApp.NewWindow("osu! launcher") // ну будем честны, это уже нихуя не патчер :3
	mainWindow.Resize(fyne.NewSize(1100, 650))
	mainWindow.SetFixedSize(true)
	mainWindow.CenterOnScreen()

	liveCanvas = container.NewWithoutLayout()

	initDataPaths() // resolves themeDir / settingsPath / themesRoot under <UserConfigDir>/osu-patcher
	loadTranslations()
	loadAppSettings()
	initDefaultTheme()

	switchToLauncher()

	// Startup decodes theme art and seeds preset themes, which roughly triples
	// the heap for a moment. Hand that back rather than sitting on it for the
	// rest of the session — this window is open while the game runs.
	go func() {
		time.Sleep(2 * time.Second)
		debug.FreeOSMemory()
	}()

	mainWindow.ShowAndRun()
}
