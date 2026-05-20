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
	"path/filepath"
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

var translations = map[string]map[string]string{
	"English": {
		"menu_file":       "File",
		"menu_save":       "Save",
		"menu_templ":      "Templates",
		"menu_exit":       "Exit Editor",
		"settings_title":  "Settings",
		"ui_tab":          "UI",
		"configs_tab":     "Configs",
		"credits_tab":     "Credits",
		"lang_label":      "Language",
		"game_path_label": "Path to osu!.exe",
		"save_path_btn":   "Save Path",
		"browse_btn":      "Browse...",
		"open_editor":     "Open UI Editor",
		"confirm_delete":  "Delete element",
		"saved_msg":       "Configuration saved successfully",
		"export_msg":      "Theme exported as .patchercfg",
		"new_theme":       "New Theme",
		"create_confirm":  "Create new theme? All current changes will be lost.",
		"back":            "Back",
	},
	"Русский": {
		"menu_file":       "Файл",
		"menu_save":       "Сохранить",
		"menu_templ":      "Шаблоны",
		"menu_exit":       "Выйти из редактора",
		"settings_title":  "Настройки",
		"ui_tab":          "Интерфейс",
		"configs_tab":     "Конфиги",
		"credits_tab":     "Авторы",
		"lang_label":      "Язык",
		"game_path_label": "Путь к osu!.exe",
		"save_path_btn":   "Сохранить путь",
		"browse_btn":      "Обзор...",
		"open_editor":     "Открыть редактор UI",
		"confirm_delete":  "Удалить элемент",
		"saved_msg":       "Конфигурация успешно сохранена",
		"export_msg":      "Тема экспортирована как .patchercfg",
		"new_theme":       "Новая тема",
		"create_confirm":  "Создать новую тему? Все изменения будут потеряны.",
		"back":            "Назад",
	},
}

func T(key string) string {
	lang := appSettings.Language
	if lang == "" {
		lang = "English"
	}
	if val, ok := translations[lang][key]; ok {
		return val
	}
	return key
}

type UIElement struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Text       string  `json:"text,omitempty"`
	Action     string  `json:"action,omitempty"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
	Width      float32 `json:"width,omitempty"`
	Height     float32 `json:"height,omitempty"`
	FontSize   float32 `json:"font_size,omitempty"`
	Opacity    float64 `json:"opacity"`
	Blur       float64 `json:"blur,omitempty"`
	Image      string  `json:"image,omitempty"`
	Endpoint   string  `json:"endpoint,omitempty"`
	ColorR     uint8   `json:"color_r"`
	ColorG     uint8   `json:"color_g"`
	ColorB     uint8   `json:"color_b"`
	TextColorR uint8   `json:"text_color_r"`
	TextColorG uint8   `json:"text_color_g"`
	TextColorB uint8   `json:"text_color_b"`
	ModuleMode string  `json:"module_mode,omitempty"`
}

type ThemeConfig struct {
	Name     string       `json:"name"`
	Elements []*UIElement `json:"elements"`
}

type AppSettings struct {
	Language string `json:"language"`
	GamePath string `json:"game_path"`
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

	themeDir     = filepath.Join("themes", "ActiveTheme")
	settingsPath = "settings.json"
)

var (
	colorTopBar   = color.NRGBA{R: 12, G: 12, B: 16, A: 255}
	colorSidebar  = color.NRGBA{R: 18, G: 18, B: 24, A: 255}
	colorSettings = color.NRGBA{R: 15, G: 15, B: 20, A: 255}
)

func loadAppSettings() {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		appSettings = AppSettings{Language: "English"}
		return
	}
	json.Unmarshal(data, &appSettings)
}

func saveAppSettings() {
	data, _ := json.MarshalIndent(appSettings, "", "  ")
	os.WriteFile(settingsPath, data, 0644)
}

func ensureThemeDir() { os.MkdirAll(themeDir, 0755) }

func configPath() string { return filepath.Join(themeDir, "config.json") }

func saveConfig() {
	ensureThemeDir()
	data, err := json.MarshalIndent(activeTheme, "", "  ")
	if err != nil {
		dialog.ShowError(err, mainWindow)
		return
	}
	if err := os.WriteFile(configPath(), data, 0644); err != nil {
		dialog.ShowError(err, mainWindow)
		return
	}
	dialog.ShowInformation(T("menu_save"), T("saved_msg"), mainWindow)
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
			dialog.ShowError(err, mainWindow)
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
		dialog.ShowInformation("Export", T("export_msg"), mainWindow)
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
		bgRect.CornerRadius = 6
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

	case "module": //TODO: webview fix. add more api patterns for diff servers than akatsuki's one. pre desing for api output
		bgRect := canvas.NewRectangle(color.NRGBA{
			R: el.ColorR, G: el.ColorG, B: el.ColorB, A: alpha,
		})
		bgRect.CornerRadius = 8

		w, h := el.Width, el.Height
		if w <= 0 { w = 200 }
		if h <= 0 { h = 80 }

		var modContainer *fyne.Container

		if el.ModuleMode == "url" && el.Endpoint != "" {

			if !isEditMode {
				webImg := StartModuleWebView(el.ID, el.Endpoint, int(w), int(h))
				webImg.Resize(fyne.NewSize(w, h))
				tap := newTappableContainer(webImg, func() { OpenEmbeddedWebView(el.Endpoint, 1000, 700) })
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

	dialog.ShowInformation("API Module", "Endpoint: "+endpoint, mainWindow)
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
		fmt.Println("(not working rn) launching osu!.exe from:", appSettings.GamePath)
		return
	}
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
		setInspectorContent(widget.NewLabel("select element"))
		return
	}
	var items []fyne.CanvasObject
	title := widget.NewLabelWithStyle(
		fmt.Sprintf("[%s] %s", strings.ToUpper(el.Type), el.ID),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)
	items = append(items, title, widget.NewSeparator())
	switch el.Type {
	case "background":
		items = append(items, buildBackgroundInspector(el)...)
	case "text":
		items = append(items, buildTextInspector(el)...)
	case "button":
		items = append(items, buildButtonInspector(el)...)
	case "module":
		items = append(items, buildModuleInspector(el)...)
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
				dialog.ShowError(err, mainWindow)
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
	actionEntry.SetPlaceHolder("https://... or internal://launch")
	actionEntry.OnChanged = func(s string) { el.Action = s }
	out := []fyne.CanvasObject{
		labeledRow("X", float32Entry(el.X, func(v float32) { el.X = v })),
		labeledRow("Y", float32Entry(el.Y, func(v float32) { el.Y = v })),
		labeledRow("Width", float32Entry(el.Width, func(v float32) { el.Width = v })),
		labeledRow("Height", float32Entry(el.Height, func(v float32) { el.Height = v })),
		labeledRow("Opacity (0-1)", floatEntry(el.Opacity, 0, 1, func(v float64) { el.Opacity = v })),
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

	updateModeButtons := func() {
		if el.ModuleMode == "url" {
			infoBtn.Importance = widget.LowImportance
			webBtn.Importance = widget.HighImportance
		} else {
			infoBtn.Importance = widget.HighImportance
			webBtn.Importance = widget.LowImportance
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
		widget.NewSeparator(),
		labeledRow("Endpoint", epEntry),
		widget.NewSeparator(),
		modeLabel,
		modeToggle,
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

func deleteElement(el *UIElement) {
	dialog.ShowConfirm(T("confirm_delete"), fmt.Sprintf("Удалить '%s'?", el.ID), func(ok bool) {
		if !ok {
			return
		}
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
	}, mainWindow)
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
	}
	var btns []fyne.CanvasObject
	btns = append(btns,
		widget.NewLabelWithStyle("Add Element", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
	)
	for _, t := range templates {
		t := t
		btns = append(btns, widget.NewButton("+ "+t.Name, func() { addElement(t.Builder()) }))
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
	header := widget.NewLabelWithStyle("Layers", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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
			dialog.ShowConfirm(T("new_theme"), T("create_confirm"), func(ok bool) {
				if ok {
					activeTheme = ThemeConfig{Name: "New Theme", Elements: defaultElements()}
					switchToEditor()
				}
			}, mainWindow)
		}),
		widget.NewButton("Open Config...", func() {
			fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
				if err != nil || uc == nil {
					return
				}
				defer uc.Close()
				data, err := io.ReadAll(uc)
				if err != nil {
					dialog.ShowError(err, mainWindow)
					return
				}
				var cfg ThemeConfig
				if err := json.Unmarshal(data, &cfg); err != nil {
					dialog.ShowError(err, mainWindow)
					return
				}
				activeTheme = cfg
				switchToEditor()
			}, mainWindow)
			fd.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
			fd.Show()
		}),
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

func buildSettingsScreen() {
	currentLang := appSettings.Language

	langSelect := widget.NewSelect([]string{"English", "Русский"}, nil)
	langSelect.SetSelected(currentLang)
	langSelect.OnChanged = func(s string) {
		if s == appSettings.Language {
			return
		}
		appSettings.Language = s
		saveAppSettings()
		buildSettingsScreen()
	}

	openEditorBtn := widget.NewButton(T("open_editor"), func() {
		switchToEditor()
	})

	uiContent := container.NewVBox(
		sectionTitle("UI"),
		widget.NewSeparator(),
		labeledRow(T("lang_label"), langSelect),
		widget.NewSeparator(),
		sectionTitle("Editor"),
		openEditorBtn,
	)

	gamePathEntry := widget.NewEntry()
	gamePathEntry.SetText(appSettings.GamePath)
	gamePathEntry.SetPlaceHolder("C:\\osu!\\osu!.exe")
	gamePathEntry.OnChanged = func(s string) { appSettings.GamePath = s }

	browseBtn := widget.NewButton(T("browse_btn"), func() {
		fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			appSettings.GamePath = uc.URI().Path()
			gamePathEntry.SetText(appSettings.GamePath)
			saveAppSettings()
		}, mainWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".exe"}))
		fd.Show()
	})

	savePathBtn := widget.NewButton(T("save_path_btn"), func() {
		saveAppSettings()
		dialog.ShowInformation("Saved", T("saved_msg"), mainWindow)
	})

	configsContent := container.NewVBox(
		sectionTitle("Game Path"),
		widget.NewSeparator(),
		widget.NewLabel(T("game_path_label")+":"),
		gamePathEntry,
		container.NewHBox(browseBtn, savePathBtn),
	)

	creditsContent := container.NewVBox(
		sectionTitle("Credits"),
		widget.NewSeparator(),

		widget.NewLabelWithStyle("taziksfear", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Main Developer"),
		makeLinkButton("github.com/taziksfear", "https://github.com/taziksfear"),

		widget.NewSeparator(),

		widget.NewLabelWithStyle("SimplyAe", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Helped with UI and fixing patcher bridge for additional clients"),
		makeLinkButton("github.com/SimplyAe", "https://github.com/SimplyAe"),
	)

	tabs := container.NewAppTabs(
		container.NewTabItem(T("ui_tab"), container.NewPadded(container.NewVScroll(uiContent))),
		container.NewTabItem(T("configs_tab"), container.NewPadded(container.NewVScroll(configsContent))),
		container.NewTabItem(T("credits_tab"), container.NewPadded(container.NewVScroll(creditsContent))),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	backBtn := widget.NewButtonWithIcon(T("back"), theme.NavigateBackIcon(), switchToLauncher)

	titleLabel := widget.NewLabelWithStyle(
		T("settings_title"),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	topBar := container.NewBorder(nil, nil, backBtn, nil, titleLabel)
	topBarBg := canvas.NewRectangle(colorTopBar)
	topBarFull := container.NewStack(topBarBg, container.NewPadded(topBar))

	bg := canvas.NewRectangle(colorSettings)
	mainWindow.SetContent(container.NewStack(
		bg,
		container.NewBorder(topBarFull, nil, nil, nil, tabs),
	))
}

func sectionTitle(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

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
	inspectorPanel = container.NewVBox(widget.NewLabel("Выберите элемент"))

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

	loadAppSettings()
	initDefaultTheme()

	switchToLauncher()
	mainWindow.ShowAndRun()
}
