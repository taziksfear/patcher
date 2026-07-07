package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	OsuFolder     string `json:"osu_folder"`   // folder where our patcher gets dropped as osu!.exe (the wine / start-menu entrypoint)
	RealOsuDir    string `json:"real_osu_dir"` // full path to the dir holding the user's actual osu!.exe; we append "osu!.exe" to it. Optional — when empty, the patcher falls back to OsuFolder/real_osu/ then OsuFolder/osu!.exe.
	Server        string `json:"server"`       // -devserver value (e.g. "akatsuki.gg", "bancho")
	Client        string `json:"client"`       // clients/<name>/ folder or "default" for real_osu
	TopPlaysURL   string `json:"top_plays_url"`
	LaunchCommand string `json:"launch_command"`
}

type ServerPreset struct {
	Name        string // human label
	DevServer   string // -devserver value / server.txt content / clients/<name> folder
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
)

func loadAppSettings() {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		appSettings = AppSettings{Language: "English"}
		return
	}
	json.Unmarshal(data, &appSettings)

	// Migration: the Game settings page used to write to GamePath (the path to
	// osu!.exe). The launch pipeline only ever reads OsuFolder. Anyone whose
	// last action was on the now-removed Game page would otherwise be silently
	// unable to launch. Promote the dir part of GamePath into OsuFolder so it
	// just works on next start, then clear the dead field.
	if strings.TrimSpace(appSettings.OsuFolder) == "" && strings.TrimSpace(appSettings.GamePath) != "" {
		appSettings.OsuFolder = filepath.Dir(normalizePath(appSettings.GamePath))
		appSettings.GamePath = ""
		saveAppSettings()
	}
}

func saveAppSettings() {
	data, _ := json.MarshalIndent(appSettings, "", "  ")
	os.WriteFile(settingsPath, data, 0644)
}

func ensureThemeDir() { os.MkdirAll(themeDir, 0755) }

func configPath() string { return filepath.Join(themeDir, "config.json") }

func saveConfig() {
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
		notifyError(err)
		return
	}
	if err := os.WriteFile(configPath(), data, 0644); err != nil {
		notifyError(err)
		return
	}
	toastSaved("theme")
}

func loadConfig() error {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &activeTheme)
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
			if _, err := os.Stat(imgPath); err == nil {
				img := canvas.NewImageFromFile(imgPath)
				img.FillMode = canvas.ImageFillStretch
				img.Translucency = 1.0 - el.Opacity
				img.Resize(winSize)
				img.Move(fyne.NewPos(0, 0))
				bg = img
				go func() {
					time.Sleep(50 * time.Millisecond)
					canvas.Refresh(img)
				}()
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
		if w <= 0 { w = 440 }
		if h <= 0 { h = 400 }
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
		if w <= 0 { w = 200 }
		if h <= 0 { h = 80 }

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
				if el.Endpoint == "" { txt = "{ API: empty}" }
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
	if el.Action == "internal://switch_client" {
		showClientPicker()
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

func showClientPicker() {
	if strings.TrimSpace(appSettings.OsuFolder) == "" && strings.TrimSpace(appSettings.PatcherPath) == "" {
		showModal(modalWarn, "No osu! folder set",
			"Set the osu! folder in Settings → Server first.\nClients are looked up under <osu_folder>/clients/.")
		return
	}

	clients, root := scanClients()

	currentLbl := widget.NewLabel("current: " + currentServerLabel())
	rootLbl := widget.NewLabel("scanning: " + root)
	rootLbl.Wrapping = fyne.TextWrapWord

	var items []fyne.CanvasObject
	items = append(items, currentLbl, rootLbl, widget.NewSeparator())

	var dlg dialog.Dialog

	defaultBtn := widget.NewButton("Default (real_osu)  —  vanilla, injected on private server", func() {
		appSettings.Client = "default"
		saveAppSettings()
		if dlg != nil {
			dlg.Hide()
		}
		toast(toastOpts{level: modalSuccess, message: "Client: vanilla (real_osu)"})
	})
	defaultBtn.Alignment = widget.ButtonAlignLeading
	items = append(items, defaultBtn)

	if len(clients) == 0 {
		items = append(items, widget.NewSeparator(),
			widget.NewLabel("No subfolders found.\nDrop a folder under the path above, with osu!.exe inside."))
	} else {
		items = append(items, widget.NewSeparator(), widget.NewLabelWithStyle("Available clients", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, name := range clients {
			name := name
			hasExe := clientHasOsuExe(root, name)
			label := name
			if !hasExe {
				label = name + "   ⚠ no osu!.exe"
			}
			btn := widget.NewButton(label, func() {
				appSettings.Client = name
				saveAppSettings()
				if dlg != nil {
					dlg.Hide()
				}
				if hasExe {
					toast(toastOpts{level: modalSuccess, message: "Client: " + name})
				} else {
					showModal(modalWarn, "Selected '"+name+"'",
						"No osu!.exe in that folder — launch will fail. Pick a different client or drop osu!.exe in.")
				}
			})
			btn.Alignment = widget.ButtonAlignLeading
			items = append(items, btn)
		}
	}

	content := container.NewVScroll(container.NewVBox(items...))
	content.SetMinSize(fyne.NewSize(420, 360))
	dlg = dialog.NewCustom("Switch Client", "Cancel", content, mainWindow)
	dlg.Show()
}

func currentServerLabel() string {
	srv := strings.TrimSpace(appSettings.Server)
	if srv == "" {
		srv = "bancho"
	}
	for _, p := range serverPresets {
		if p.DevServer == srv {
			srv = p.Name
			break
		}
	}
	client := strings.TrimSpace(appSettings.Client)
	if client == "" || client == "default" {
		client = "vanilla"
	}
	return srv + "  ·  client: " + client
}

// ── launch / inject pipeline ────────────────────────────────────────────────
//
// This is the proven flow from example/GO:
//
//   1. Resolve the osu! folder root (the dir that holds real_osu/, clients/,
//      and server.txt — NOT real_osu/ itself).
//   2. If the user hasn't scaffolded yet (no real_osu/ but loose game files at
//      the top), prompt to move them in.
//   3. Drop the bundled patcher exe AS "osu!.exe" at the osu root. osu-wine
//      will launch that file, so the patcher gets control before the real
//      game starts.
//   4. Write server.txt + client.txt next to it so the patcher knows where to
//      route. (The patcher also accepts --osu-dir/--server/--client CLI args
//      for standalone use.)
//   5. Launch via osu-wine (Linux) or via the patcher exe directly (Windows).
//      A user-supplied LaunchCommand overrides everything.

const patcherFilename = "osu!.exe"

// Files we own at the top of the osu folder; never move these into real_osu/.
var ownTopFiles = map[string]bool{
	patcherFilename:      true,
	"real_osu":           true,
	"clients":            true,
	"server.txt":         true,
	"client.txt":         true,
	"custom_servers.txt": true,
	"patcher_log.txt":    true,
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
	rawSetting := strings.TrimSpace(appSettings.OsuFolder)
	osuDir := normalizePath(rawSetting)

	// First-run convenience: rather than dropping a wall-of-text error on
	// someone hitting Launch on a clean install, open the folder picker
	// directly. After they pick, we save it and continue the launch.
	if osuDir == "" {
		promptOsuFolderThenLaunch()
		return
	}
	if _, err := os.Stat(osuDir); err != nil {
		showModal(modalError, "osu! folder not found",
			fmt.Sprintf("The saved folder doesn't exist on disk:\n  %s\n\n(raw value: %q)\n\nPick it again?", osuDir, rawSetting),
			modalAction{Label: "Cancel"},
			modalAction{Label: "Pick folder…", Primary: true, OnClick: promptOsuFolderThenLaunch},
		)
		return
	}
	osuDir = resolveOsuRoot(osuDir)

	// custom launch command short-circuits the whole pipeline
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

	if scaffNeeded, movable := needsScaffold(osuDir); scaffNeeded {
		msg := fmt.Sprintf(
			"First-time setup at:\n%s\n\nMove %d existing items into real_osu/ so the patcher can take over osu!.exe?\n\nItems: %s",
			osuDir, len(movable), strings.Join(movable, ", "),
		)
		confirmModal("Scaffold osu folder", msg, "Move & Launch", func() {
			if err := scaffold(osuDir, movable); err != nil {
				notifyError(err)
				return
			}
			finishLaunch(osuDir)
		})
		return
	}
	_ = os.MkdirAll(filepath.Join(osuDir, "clients"), 0755)
	finishLaunch(osuDir)
}

func finishLaunch(osuDir string) {
	srv := strings.TrimSpace(appSettings.Server)
	if srv == "" {
		srv = "bancho"
	}
	client := strings.TrimSpace(appSettings.Client)
	if client == "" {
		client = "default"
	}

	// 1. drop the patcher exe AS osu!.exe so osu-wine picks it up
	if err := installPatcherExe(osuDir); err != nil {
		notifyError(fmt.Errorf("install patcher: %v", err))
		return
	}
	// 2. selection files (also let the patcher run standalone)
	if err := os.WriteFile(filepath.Join(osuDir, "server.txt"), []byte(srv), 0644); err != nil {
		notifyError(err)
		return
	}
	if err := os.WriteFile(filepath.Join(osuDir, "client.txt"), []byte(client), 0644); err != nil {
		notifyError(err)
		return
	}

	// 3. launch via osu-wine (best wine prefix handling), windows native, or wine fallback
	cmd, source, err := launchCommand(osuDir)
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
	fmt.Printf("[Launch] %s (pid %d) | server=%s client=%s osuDir=%s\n",
		source, cmd.Process.Pid, srv, client, osuDir)
}

// installPatcherExe writes the embedded patcher into osuDir/osu!.exe.
// Falls back to the user-configured PatcherPath if no bundled exe is present.
// If the existing osu!.exe at that location is byte-identical we skip the write.
func installPatcherExe(osuDir string) error {
	dst := filepath.Join(osuDir, patcherFilename)

	if hasEmbeddedPatcher() {
		if existing, err := os.ReadFile(dst); err == nil && bytesEqual(existing, embeddedPatcherExe) {
			return nil
		}
		return os.WriteFile(dst, embeddedPatcherExe, 0755)
	}

	src := normalizePath(strings.TrimSpace(appSettings.PatcherPath))
	if src == "" {
		return fmt.Errorf("no bundled patcher in this build and no PatcherPath set — rebuild via build.sh or set Settings → Server & Client → Advanced")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %v", src, err)
	}
	if existing, err := os.ReadFile(dst); err == nil && bytesEqual(existing, data) {
		return nil
	}
	return os.WriteFile(dst, data, 0755)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// launchCommand returns the exec.Cmd that actually fires up the patcher.
//   Linux: prefer osu-wine (handles WINEPREFIX/WINEARCH/dxvk); fall back to
//          plain wine. Either way we point it at the patcher exe in osuDir.
//   Windows: just run the exe.
func launchCommand(osuDir string) (*exec.Cmd, string, error) {
	patcher := filepath.Join(osuDir, patcherFilename)

	if runtime.GOOS == "windows" {
		c := exec.Command(patcher)
		c.Dir = osuDir
		c.Env = os.Environ()
		return c, patcher, nil
	}

	for _, cand := range []string{"osu-wine", filepath.Join(os.Getenv("HOME"), ".local/bin/osu-wine")} {
		if bin, err := resolveBinary(cand); err == nil {
			c := exec.Command(bin)
			c.Dir = osuDir
			c.Env = os.Environ()
			return c, bin, nil
		}
	}
	wine, err := resolveBinary("wine")
	if err != nil {
		return nil, "", fmt.Errorf("neither osu-wine nor wine found in PATH")
	}
	c := exec.Command(wine, patcher)
	c.Dir = osuDir
	c.Env = os.Environ()
	return c, "wine " + patcher, nil
}

// needsScaffold returns true when osu dir has game files at the top level but
// no real_osu/ subdir — meaning we should offer to move them in so the patcher
// can take over osu!.exe at the root without colliding with the real game.
func needsScaffold(osuDir string) (bool, []string) {
	if dirExists(filepath.Join(osuDir, "real_osu")) {
		return false, nil
	}
	entries, err := os.ReadDir(osuDir)
	if err != nil {
		return false, nil
	}
	var movable []string
	for _, e := range entries {
		if ownTopFiles[e.Name()] {
			continue
		}
		movable = append(movable, e.Name())
	}
	return len(movable) > 0, movable
}

func scaffold(osuDir string, movable []string) error {
	realOsu := filepath.Join(osuDir, "real_osu")
	if err := os.MkdirAll(realOsu, 0755); err != nil {
		return err
	}
	for _, name := range movable {
		src := filepath.Join(osuDir, name)
		dst := filepath.Join(realOsu, name)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move %s: %w", name, err)
		}
	}
	return os.MkdirAll(filepath.Join(osuDir, "clients"), 0755)
}

// resolveClientsRoot finds the clients/ directory regardless of whether the
// patcher exe is placed next to it OR inside one of its subfolders.
// Search order: <dir>/clients → walk up; if a parent is itself named "clients",
// use that one. Falls back to <patcherDir>/clients even if it doesn't exist.
func resolveClientsRoot(patcher string) string {
	return resolveClientsRootFromDir(filepath.Dir(patcher))
}

// resolveOsuRoot finds the osu! root that holds clients/ (and usually real_osu/).
// Walks up from whatever the user picked, looking specifically for a clients/
// subdir — that's the unambiguous marker of the actual root, vs. real_osu/
// which also has an osu!.exe and would falsely match if we treated osu!.exe as
// a root marker.
func resolveOsuRoot(start string) string {
	dir := start
	for i := 0; i < 6; i++ {
		if dirExists(filepath.Join(dir, "clients")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return start
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// resolveClientsRootFromDir does the same walk-up but starting from an
// arbitrary directory — used so OsuFolder can point at either the parent of
// real_osu/ or at real_osu/ itself.
func resolveClientsRootFromDir(start string) string {
	dir := start
	for i := 0; i < 6; i++ {
		if filepath.Base(dir) == "clients" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
		candidate := filepath.Join(dir, "clients")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join(start, "clients")
}

// scanClients returns subfolder names under <osuFolder>/clients/.
// Falls back to walking up from PatcherPath when OsuFolder is not set, for the
// manual-override case.
func scanClients() (folders []string, root string) {
	if osuDir := normalizePath(strings.TrimSpace(appSettings.OsuFolder)); osuDir != "" {
		root = resolveClientsRootFromDir(osuDir)
	} else {
		patcher := normalizePath(strings.TrimSpace(appSettings.PatcherPath))
		if patcher == "" {
			return nil, ""
		}
		root = resolveClientsRoot(patcher)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Printf("[clients] cannot read %s: %v\n", root, err)
		return nil, root
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		folders = append(folders, e.Name())
	}
	return folders, root
}

func clientHasOsuExe(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name, "osu!.exe"))
	return err == nil
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
	actionEntry.SetPlaceHolder("https://... or internal://launch | switch_server | switch_client")
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
		{"Switch Client", func() *UIElement {
			return &UIElement{
				ID: nextID("btn_client"), Type: "button",
				Text: "🎮  Switch Client", X: 60, Y: 410,
				Width: 220, Height: 44, Opacity: 1.0,
				ColorR: 90, ColorG: 60, ColorB: 140,
				TextColorR: 255, TextColorG: 255, TextColorB: 255,
				Action: "internal://switch_client",
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
		{"appearance", "Appearance", "Language, theme and editor access", theme.ColorPaletteIcon(), buildAppearancePage},
		{"themes", "Themes", "Pick a preloaded look or apply your own", theme.GridIcon(), buildThemesPage},
		{"account", "Account", "Connect your top plays via a private server API", theme.AccountIcon(), buildAccountPage},
		{"server", "Server & Client", "osu! folder + which server / custom client gets launched", theme.ComputerIcon(), buildServerPage},
		{"launch", "Launch Command", "Manually override how osu! is started", theme.MailForwardIcon(), buildLaunchPage},
		{"credits", "Credits", "Who built this", theme.InfoIcon(), buildCreditsPage},
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
		canvas.NewText("SETTINGS", colorAccent),
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
	userEntry.OnChanged = func(s string) { appSettings.UserName = s }
	userSaveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		saveAppSettings()
		toastSaved("display name")
	})

	openEditorBtn := widget.NewButtonWithIcon(T("open_editor"), theme.DocumentCreateIcon(), switchToEditor)
	openEditorBtn.Importance = widget.HighImportance

	langCard := subCard(container.NewVBox(
		fieldLabel(strings.ToUpper(T("lang_label"))),
		langSelect,
	))

	userCard := subCard(container.NewVBox(
		fieldLabel("DISPLAY NAME"),
		widget.NewLabel("Used as the author tag on themes you save with “Save as New Theme”."),
		userEntry,
		container.NewHBox(userSaveBtn),
	))

	editorCard := subCard(container.NewVBox(
		fieldLabel("UI EDITOR"),
		widget.NewLabel("Customize the launcher layout — drag elements, add buttons, modules, top plays."),
		openEditorBtn,
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

func buildServerPage() fyne.CanvasObject {
	// ── osu! folder (preferred) ──
	osuFolderEntry := widget.NewEntry()
	osuFolderEntry.SetText(appSettings.OsuFolder)
	osuFolderEntry.SetPlaceHolder("e.g. ~/.local/share/osu-wine/osu!  or  C:\\osu!")
	// Auto-save on every keystroke so users can't get into the "I typed the
	// path but forgot to hit Save" trap — Launch was failing for new users
	// because of this. Path is normalised (file://, Windows /C:/ quirk, URI
	// decode) on read, not on save, so we keep the raw text editable.
	osuFolderEntry.OnChanged = func(s string) {
		appSettings.OsuFolder = s
		saveAppSettings()
	}

	osuFolderBrowseBtn := widget.NewButtonWithIcon(T("browse_btn"), theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			// Normalise immediately on Windows so the displayed value matches
			// what we'll actually use at launch time (no rogue leading slash).
			appSettings.OsuFolder = normalizePath(uri.Path())
			osuFolderEntry.SetText(appSettings.OsuFolder)
			saveAppSettings()
		}, mainWindow)
	})
	osuFolderSaveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		saveAppSettings()
		toastSaved("osu! folder")
	})

	bundleStatus := canvas.NewText("bundled patcher: ✓ ready", color.NRGBA{R: 80, G: 215, B: 130, A: 255})
	if !hasEmbeddedPatcher() {
		bundleStatus.Text = "bundled patcher: ✕ missing — rebuild via build.sh"
		bundleStatus.Color = color.NRGBA{R: 255, G: 130, B: 130, A: 255}
	}
	bundleStatus.TextSize = 11
	bundleStatus.TextStyle = fyne.TextStyle{Italic: true}

	osuCard := subCard(container.NewVBox(
		fieldLabel("OSU! FOLDER"),
		widget.NewLabel("The folder containing osu!.exe (and optionally clients/). The bundled patcher reads this to route launches."),
		osuFolderEntry,
		container.NewHBox(osuFolderBrowseBtn, osuFolderSaveBtn),
		bundleStatus,
	))

	// ── optional override ──
	patcherPathEntry := widget.NewEntry()
	patcherPathEntry.SetText(appSettings.PatcherPath)
	patcherPathEntry.SetPlaceHolder("(advanced) path to an external osu_patcher.exe")
	patcherPathEntry.OnChanged = func(s string) { appSettings.PatcherPath = s }

	patcherBrowseBtn := widget.NewButtonWithIcon(T("browse_btn"), theme.FolderOpenIcon(), func() {
		fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			appSettings.PatcherPath = uc.URI().Path()
			patcherPathEntry.SetText(appSettings.PatcherPath)
			saveAppSettings()
		}, mainWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".exe"}))
		fd.Show()
	})
	patcherSaveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		saveAppSettings()
		toastSaved("patcher path")
	})
	patcherClearBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		appSettings.PatcherPath = ""
		patcherPathEntry.SetText("")
		saveAppSettings()
		toast(toastOpts{level: modalSuccess, message: "patcher path cleared"})
	})

	overrideNote := canvas.NewText("Ignored when the bundled patcher is available.", colorTextMute)
	overrideNote.TextSize = 10
	overrideNote.TextStyle = fyne.TextStyle{Italic: true}

	overrideCard := subCard(container.NewVBox(
		fieldLabel("ADVANCED · CUSTOM PATCHER EXE"),
		widget.NewLabel("Point at your own externally-built osu_patcher.exe. Only used when no bundled patcher is in this build."),
		patcherPathEntry,
		container.NewHBox(patcherBrowseBtn, patcherSaveBtn, patcherClearBtn),
		overrideNote,
	))

	// ── server status + switchers ──
	currentServerTxt := canvas.NewText(currentServerLabel(), colorAccent)
	currentServerTxt.TextSize = 18
	currentServerTxt.TextStyle = fyne.TextStyle{Bold: true}

	pickServerBtn := widget.NewButtonWithIcon("Switch Server…", theme.ComputerIcon(), func() { showServerPicker() })
	pickServerBtn.Importance = widget.HighImportance
	pickClientBtn := widget.NewButtonWithIcon("Switch Client…", theme.MediaPlayIcon(), func() { showClientPicker() })

	statusCard := subCard(container.NewVBox(
		fieldLabel("CURRENT SERVER"),
		currentServerTxt,
		widget.NewSeparator(),
		container.NewHBox(pickServerBtn, pickClientBtn),
	))

	return container.NewVBox(osuCard, statusCard, overrideCard)
}

func buildLaunchPage() fyne.CanvasObject {
	launchCmdEntry := widget.NewEntry()
	launchCmdEntry.SetText(appSettings.LaunchCommand)
	if runtime.GOOS == "windows" {
		launchCmdEntry.SetPlaceHolder("leave empty to use the Patcher / Game Path")
	} else {
		launchCmdEntry.SetPlaceHolder("leave empty for auto (Patcher exe → osu-wine → wine <path>)")
	}
	launchCmdEntry.OnChanged = func(s string) { appSettings.LaunchCommand = s }

	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		saveAppSettings()
		toastSaved("launch command")
	})
	testBtn := widget.NewButtonWithIcon("Test launch", theme.MediaPlayIcon(), func() { launchOsu() })
	testBtn.Importance = widget.HighImportance

	examples := canvas.NewText("Examples:  osu-wine  |  wine ~/.local/share/osu-wine/osu!/osu!.exe  |  lutris lutris:rungame/osu-stable", colorTextMute)
	examples.TextSize = 10

	card := subCard(container.NewVBox(
		fieldLabel("CUSTOM COMMAND"),
		widget.NewLabel("Optional override — completely replaces the launch logic with whatever you type here."),
		launchCmdEntry,
		container.NewHBox(saveBtn, testBtn),
		examples,
	))

	return container.NewVBox(card)
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
		person("SimplyAe", "Helped with UI and fixing the patcher bridge for additional clients", "https://github.com/SimplyAe"),
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
		return color.NRGBA{R: 80, G: 215, B: 130, A: 255}
	case modalError:
		return color.NRGBA{R: 255, G: 80, B: 100, A: 255}
	case modalWarn:
		return color.NRGBA{R: 245, G: 190, B: 70, A: 255}
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

func main() {
	currentApp = app.New()
	currentApp.Settings().SetTheme(theme.DarkTheme())

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
	mainWindow.ShowAndRun()
}