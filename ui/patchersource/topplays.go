package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const (
	coverURLFormat = "https://assets.ppy.sh/beatmaps/%d/covers/cover.jpg"
	maxTopPlays    = 5
)

type TopPlaysPreset struct {
	Name        string
	URLTemplate string // single %s for user id
}

var topPlaysPresets = []TopPlaysPreset{
	{"Akatsuki (vanilla)", "https://akatsuki.gg/api/v1/users/scores/best?id=%s&mode=0&rx=0&l=5"},
	{"Akatsuki (relax)", "https://akatsuki.gg/api/v1/users/scores/best?id=%s&mode=0&rx=1&l=5"},
	{"Akatsuki (autopilot)", "https://akatsuki.gg/api/v1/users/scores/best?id=%s&mode=0&rx=2&l=5"},
	{"Ripple", "https://ripple.moe/api/v1/users/scores/best?id=%s&mode=0&rx=0&l=5"},
	{"Ripple (relax)", "https://ripple.moe/api/v1/users/scores/best?id=%s&mode=0&rx=1&l=5"},
}

type TopBeatmap struct {
	BeatmapID    int     `json:"beatmap_id"`
	BeatmapSetID int     `json:"beatmapset_id"`
	SongName     string  `json:"song_name"`
	Difficulty   float64 `json:"difficulty"`
}

type TopScore struct {
	PP        float64    `json:"pp"`
	Accuracy  float64    `json:"accuracy"`
	Rank      string     `json:"rank"`
	MaxCombo  int        `json:"max_combo"`
	FullCombo bool       `json:"full_combo"`
	Mods      int        `json:"mods"`
	Beatmap   TopBeatmap `json:"beatmap"`
}

type TopPlaysResponse struct {
	Code   int        `json:"code"`
	Scores []TopScore `json:"scores"`
}

var (
	coverCache   = map[int]string{}
	coverCacheMu sync.Mutex
)

func downloadCover(beatmapSetID int) (string, error) {
	coverCacheMu.Lock()
	if p, ok := coverCache[beatmapSetID]; ok {
		coverCacheMu.Unlock()
		return p, nil
	}
	coverCacheMu.Unlock()

	dest := filepath.Join(os.TempDir(), fmt.Sprintf("patcher_cover_%d.jpg", beatmapSetID))

	if _, err := os.Stat(dest); err == nil {
		coverCacheMu.Lock()
		coverCache[beatmapSetID] = dest
		coverCacheMu.Unlock()
		return dest, nil
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(fmt.Sprintf(coverURLFormat, beatmapSetID))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(dest)
		return "", err
	}

	coverCacheMu.Lock()
	coverCache[beatmapSetID] = dest
	coverCacheMu.Unlock()

	return dest, nil
}


func fetchTopPlays(endpoint string) ([]TopScore, error) {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network: %w", err)
	}
	defer resp.Body.Close()

	var result TopPlaysResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("bad JSON: %w", err)
	}
	if result.Code != 200 {
		return nil, fmt.Errorf("API code %d", result.Code)
	}
	n := len(result.Scores)
	if n > maxTopPlays {
		n = maxTopPlays
	}
	return result.Scores[:n], nil
}

var modBits = []struct {
	bit  int
	name string
}{
	{1, "NF"}, {2, "EZ"}, {8, "HD"}, {16, "HR"},
	{64, "DT"}, {512, "NC"}, {1024, "FL"}, {16384, "PF"},
	{128, "RX"}, {2048, "AP"},
}

func modsString(mods int) string {
	if mods == 0 {
		return "NM"
	}
	var b strings.Builder
	for _, m := range modBits {
		if mods&m.bit != 0 {
			b.WriteString(m.name)
		}
	}
	if b.Len() == 0 {
		return "NM"
	}
	return b.String()
}

func rankGradeColor(rank string) color.NRGBA {
	switch rank {
	case "XH":
		return color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	case "X":
		return color.NRGBA{R: 255, G: 215, B: 0, A: 255}
	case "SH":
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	case "S":
		return color.NRGBA{R: 255, G: 200, B: 40, A: 255}
	case "A":
		return color.NRGBA{R: 80, G: 210, B: 100, A: 255}
	case "B":
		return color.NRGBA{R: 80, G: 150, B: 255, A: 255}
	case "C":
		return color.NRGBA{R: 180, G: 80, B: 255, A: 255}
	default: // D
		return color.NRGBA{R: 255, G: 80, B: 80, A: 255}
	}
}

func truncateRune(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func buildPlayCard(idx int, score TopScore, cardW, cardH float32) fyne.CanvasObject {
	const rightW = 0.26

	coverImg := canvas.NewImageFromFile("")
	coverImg.FillMode = canvas.ImageFillStretch
	coverImg.SetMinSize(fyne.NewSize(cardW, cardH))
	coverImg.Resize(fyne.NewSize(cardW, cardH))

	fullDim := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 120})

	pink := color.NRGBA{R: 255, G: 100, B: 165, A: 210}

	indexLbl := canvas.NewText(fmt.Sprintf("#%d", idx+1), pink)
	indexLbl.TextSize = 9
	indexLbl.TextStyle = fyne.TextStyle{Bold: true}

	nameLbl := canvas.NewText(
		truncateRune(score.Beatmap.SongName, 44),
		color.NRGBA{R: 255, G: 255, B: 255, A: 245},
	)
	nameLbl.TextSize = 12
	nameLbl.TextStyle = fyne.TextStyle{Bold: true}

	fcStr := ""
	if score.FullCombo {
		fcStr = "  ●FC"
	}
	subLbl := canvas.NewText(
		fmt.Sprintf("%.2f%%  %s%s", score.Accuracy, modsString(score.Mods), fcStr),
		color.NRGBA{R: 175, G: 175, B: 200, A: 200},
	)
	subLbl.TextSize = 10

	leftVBox := container.NewVBox(indexLbl, nameLbl, subLbl)
	leftPad := container.NewPadded(leftVBox)

	rankLbl := canvas.NewText(score.Rank, rankGradeColor(score.Rank))
	rankLbl.TextSize = 19
	rankLbl.TextStyle = fyne.TextStyle{Bold: true}
	rankLbl.Alignment = fyne.TextAlignCenter

	ppLbl := canvas.NewText(
		fmt.Sprintf("%.0fpp", score.PP),
		color.NRGBA{R: 255, G: 145, B: 200, A: 255},
	)
	ppLbl.TextSize = 13
	ppLbl.TextStyle = fyne.TextStyle{Bold: true}
	ppLbl.Alignment = fyne.TextAlignCenter

	rightVBox := container.NewVBox(rankLbl, ppLbl)
	rightCentered := container.NewCenter(rightVBox)

	row := container.NewBorder(
		nil, nil, nil,
		container.New(&fixedWidthLayout{width: cardW * rightW}, rightCentered),
		leftPad,
	)

	card := container.NewStack(coverImg, fullDim, row)

	go func(setID int) {
		if setID <= 0 {
			return
		}
		path, err := downloadCover(setID)
		if err != nil {
			fmt.Printf("[TopPlays] cover %d: %v\n", setID, err)
			return
		}
		time.Sleep(15 * time.Millisecond)
		coverImg.File = path
		coverImg.Resize(fyne.NewSize(cardW, cardH))
		coverImg.Refresh()
		canvas.Refresh(coverImg)
	}(score.Beatmap.BeatmapSetID)

	return card
}

func buildTopPlaysSetupView(el *UIElement) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.NRGBA{R: 16, G: 13, B: 26, A: 255})
	bg.CornerRadius = 8

	titleLbl := canvas.NewText("connect your account", color.NRGBA{R: 255, G: 100, B: 165, A: 255})
	titleLbl.TextSize = 15
	titleLbl.TextStyle = fyne.TextStyle{Bold: true}
	titleLbl.Alignment = fyne.TextAlignCenter

	hintLbl := canvas.NewText("paste your top-plays API endpoint below", color.NRGBA{R: 155, G: 155, B: 185, A: 200})
	hintLbl.TextSize = 10
	hintLbl.Alignment = fyne.TextAlignCenter

	exampleLbl := canvas.NewText("e.g. …/api/v1/users/scores/best?id=1234&mode=0&rx=0&l=5", color.NRGBA{R: 120, G: 120, B: 150, A: 160})
	exampleLbl.TextSize = 9
	exampleLbl.Alignment = fyne.TextAlignCenter

	entry := widget.NewEntry()
	entry.SetPlaceHolder("https://akatsuki.gg/api/v1/users/scores/best?id=…")

	statusLbl := canvas.NewText("", color.NRGBA{R: 255, G: 100, B: 100, A: 255})
	statusLbl.TextSize = 10
	statusLbl.Alignment = fyne.TextAlignCenter

	connectBtn := widget.NewButton("Connect", func() {
		url := strings.TrimSpace(entry.Text)
		if url == "" {
			statusLbl.Text = "URL cannot be empty"
			statusLbl.Refresh()
			return
		}
		statusLbl.Text = "checking…"
		statusLbl.Color = color.NRGBA{R: 200, G: 200, B: 100, A: 255}
		statusLbl.Refresh()
		go func() {
			_, err := fetchTopPlays(url)
			if err != nil {
				statusLbl.Text = "⚠ " + err.Error()
				statusLbl.Color = color.NRGBA{R: 255, G: 100, B: 100, A: 255}
				statusLbl.Refresh()
				return
			}
			el.Endpoint = url
			saveConfig()
			buildCanvasObjects(isEditorMode)
		}()
	})

	form := container.NewVBox(
		container.NewCenter(titleLbl),
		container.NewCenter(hintLbl),
		container.NewCenter(exampleLbl),
		widget.NewSeparator(),
		entry,
		connectBtn,
		container.NewCenter(statusLbl),
	)

	return container.NewStack(bg, container.NewCenter(container.NewPadded(form)))
}

func BuildTopPlaysWidget(el *UIElement) fyne.CanvasObject {
	w, h := el.Width, el.Height
	if w <= 0 {
		w = 440
	}
	if h <= 0 {
		h = 400
	}

	if isEditorMode {
		bg := canvas.NewRectangle(color.NRGBA{R: el.ColorR, G: el.ColorG, B: el.ColorB, A: 200})
		bg.CornerRadius = 8
		label := "⯈ Top Plays module"
		if el.Endpoint != "" {
			label = "⯈ Top Plays  ·  " + el.Endpoint
		}
		lbl := canvas.NewText(truncateRune(label, 55), color.NRGBA{R: el.TextColorR, G: el.TextColorG, B: el.TextColorB, A: 200})
		lbl.TextSize = 11
		return container.NewStack(bg, container.NewCenter(lbl))
	}

	if el.Endpoint == "" {
		return buildTopPlaysSetupView(el)
	}

	bg := canvas.NewRectangle(color.NRGBA{R: 14, G: 12, B: 22, A: 255})
	bg.CornerRadius = 8

	statusLbl := canvas.NewText("loading top plays…", color.NRGBA{R: 160, G: 160, B: 200, A: 180})
	statusLbl.TextSize = 12
	statusLbl.Alignment = fyne.TextAlignCenter

	cardArea := container.NewVBox()
	outerStack := container.NewStack(bg, container.NewCenter(statusLbl), cardArea)

	go func() {
		scores, err := fetchTopPlays(el.Endpoint)
		if err != nil {
			statusLbl.Text = "⚠ " + err.Error()
			statusLbl.Color = color.NRGBA{R: 255, G: 90, B: 90, A: 255}
			statusLbl.Refresh()
			return
		}
		if len(scores) == 0 {
			statusLbl.Text = "no scores found"
			statusLbl.Refresh()
			return
		}

		cardH := h / float32(len(scores))
		var cards []fyne.CanvasObject
		for i, s := range scores {
			if i > 0 {
				sep := canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 25})
				sep.SetMinSize(fyne.NewSize(w, 1))
				cards = append(cards, sep)
			}
			card := buildPlayCard(i, s, w, cardH)
			card.Resize(fyne.NewSize(w, cardH))
			cards = append(cards, card)
		}

		statusLbl.Text = ""
		statusLbl.Refresh()
		cardArea.Objects = cards
		cardArea.Refresh()
		canvas.Refresh(outerStack)
	}()

	return outerStack
}