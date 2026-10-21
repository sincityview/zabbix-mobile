package main

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"zabbix/data"
)

var (
	shortProblem = data.Problem{
		EventID:  "1",
		Clock:    "1700000000",
		Severity: "3",
		HostName: "host-a",
		Name:     "Недоступен по протоколу ICMP",
	}
	longProblem = data.Problem{
		EventID:  "2",
		Clock:    "1700000000",
		Severity: "5",
		HostName: "ул. Индустриальное шоссе, д. 9, п. 2 [ ELTEX MES2324FB ]",
		Name:     "Слабый сигнал порт 15: -23.67 dbm, деградация канала, требуется выезд бригады на объект",
	}
)

// Регрессия на картинку 3: у widget.List высота элемента фиксированная, поэтому
// длинное имя хоста не должно увеличивать высоту карточки.
func TestProblemRow_HeightIndependentOfTextLength(t *testing.T) {
	test.NewApp()

	measure := func(p data.Problem) fyne.Size {
		row := newProblemRow()
		win := test.NewWindow(row)
		win.Resize(fyne.NewSize(360, 200))
		row.setProblem(p)
		win.Resize(fyne.NewSize(360, 200))
		size := row.MinSize()
		win.Close()
		return size
	}

	shortSize := measure(shortProblem)
	longSize := measure(longProblem)

	if shortSize.Height != longSize.Height {
		t.Fatalf("высота карточки зависит от длины текста: короткий=%.0f, длинный=%.0f",
			shortSize.Height, longSize.Height)
	}
}

// Текст в карточке должен укладываться в её ширину (иначе он уезжает за край).
func TestProblemRow_TextFitsWidth(t *testing.T) {
	test.NewApp()

	for _, width := range []float32{240, 360, 500} {
		row := newProblemRow()
		win := test.NewWindow(row)
		win.Resize(fyne.NewSize(width, 200))
		row.setProblem(longProblem)
		win.Resize(fyne.NewSize(width, 200))

		if got := row.hostLabel.MinSize().Width; got > width {
			t.Errorf("ширина %.0f: имя хоста %.0f шире карточки", width, got)
		}
		if got := row.nameLabel.MinSize().Width; got > width {
			t.Errorf("ширина %.0f: текст проблемы %.0f шире карточки", width, got)
		}
		win.Close()
	}
}

func TestProblemRow_KeepsInformativeTail(t *testing.T) {
	test.NewApp()

	row := newProblemRow()
	win := test.NewWindow(row)
	win.Resize(fyne.NewSize(300, 200))
	row.setProblem(longProblem)
	win.Resize(fyne.NewSize(300, 200))
	defer win.Close()

	host := row.hostLabel.Text
	if !strings.Contains(host, "…") {
		t.Fatalf("длинное имя хоста не обрезано: %q", host)
	}
	if !strings.Contains(host, "MES2324FB") {
		t.Errorf("при обрезке потерялась модель устройства: %q", host)
	}
	if !strings.HasPrefix(host, "ул.") {
		t.Errorf("при обрезке потерялось начало: %q", host)
	}
}

func TestFitMiddle(t *testing.T) {
	test.NewApp()

	size := theme.TextSize()
	style := fyne.TextStyle{Bold: true}
	text := "ул. Карла Маркса, д. 10, п. 6, корп. Апекс-Крым [ ELTEX MES2324FB ]"

	if got := fitMiddle(text, 10000, size, style); got != text {
		t.Errorf("широкий текст обрезан без необходимости: %q", got)
	}
	if got := fitMiddle("", 100, size, style); got != "" {
		t.Errorf("пустой текст превратился в %q", got)
	}

	for _, width := range []float32{120, 200, 320} {
		narrow := fitMiddle(text, width, size, style)
		if !strings.Contains(narrow, "…") {
			t.Errorf("ширина %.0f: текст не обрезан: %q", width, narrow)
			continue
		}
		if got := fyne.MeasureText(narrow, size, style).Width; got > width {
			t.Errorf("ширина %.0f: обрезанный текст шире лимита: %.0f", width, got)
		}
		// начало и конец должны быть фрагментами исходного текста
		head, tail, ok := splitEllipsis(narrow)
		if !ok {
			t.Errorf("ширина %.0f: нет разделителя: %q", width, narrow)
			continue
		}
		if !strings.HasPrefix(text, head) {
			t.Errorf("ширина %.0f: начало не из исходного текста: %q", width, head)
		}
		if !strings.HasSuffix(text, tail) {
			t.Errorf("ширина %.0f: конец не из исходного текста: %q", width, tail)
		}
	}

	// на реальной ширине карточки модель устройства должна сохраняться
	if got := fitMiddle(text, 300, size, style); !strings.Contains(got, "MES2324FB") {
		t.Errorf("потеряна модель устройства: %q", got)
	}
}

// splitEllipsis делит результат fitMiddle на часть до и после «…».
func splitEllipsis(s string) (head, tail string, ok bool) {
	parts := strings.SplitN(s, "…", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Регрессия: label с Truncation имеет MinSize шириной в один символ
// (widget/richtext.go, calculateMin), поэтому в HBox статус превращался в «…».
// Верхняя панель должна собираться через Border.
func TestStatusBarNotCollapsed(t *testing.T) {
	test.NewApp()

	const status = "Проблем: 12 · Новых: 2"

	newLabel := func() *widget.Label {
		l := widget.NewLabel(status)
		l.TextStyle = fyne.TextStyle{Bold: true}
		l.Truncation = fyne.TextTruncateEllipsis
		return l
	}

	// как было: HBox + Spacer — метка сжимается до минимума
	hboxLabel := newLabel()
	hbox := container.NewHBox(hboxLabel, layout.NewSpacer(), widget.NewButtonWithIcon("", theme.SettingsIcon(), nil))
	win := test.NewWindow(hbox)
	win.Resize(fyne.NewSize(320, 60))
	collapsedWidth := hboxLabel.Size().Width
	win.Close()

	// как стало: Border отдаёт центральному элементу всю оставшуюся ширину
	borderLabel := newLabel()
	topBar := container.NewBorder(nil, nil, nil, widget.NewButtonWithIcon("", theme.SettingsIcon(), nil), borderLabel)
	refreshBtn := widget.NewButton(data.Tr("update"), nil)

	// та же вложенность, что в buildContent: внешний Border + VBox сверху/снизу
	content := container.NewBorder(
		container.NewVBox(topBar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), refreshBtn),
		nil, nil, widget.NewLabel("content"),
	)
	win2 := test.NewWindow(content)
	defer win2.Close()
	// типичная логическая ширина телефона
	win2.Resize(fyne.NewSize(195, 400))
	fixedWidth := borderLabel.Size().Width

	if fixedWidth < 130 {
		t.Errorf("статус всё ещё сжат: ширина %.0f из 195", fixedWidth)
	}
	if collapsedWidth >= fixedWidth {
		t.Logf("HBox-вариант не сжался (%.0f vs %.0f) — тест всё равно валиден для Border",
			collapsedWidth, fixedWidth)
	}
}

// Разделители должны быть видны: в тёмной теме базовая тема даёт чисто чёрный
// цвет на почти чёрном фоне, поэтому карточки сливались.
func TestSeparatorThemeIsVisible(t *testing.T) {
	base := theme.DarkTheme()
	baseSep := base.Color(theme.ColorNameSeparator, theme.VariantDark)
	r, g, b, a := baseSep.RGBA()
	if a != 0xffff || r != 0 || g != 0 || b != 0 {
		t.Logf("базовый цвет разделителя изменился в fyne: %v", baseSep)
	}

	for _, light := range []bool{false, true} {
		th := appTheme(light)
		variant := theme.VariantDark
		if light {
			variant = theme.VariantLight
		}

		sep := th.Color(theme.ColorNameSeparator, variant)
		sr, sg, sb, sa := sep.RGBA()
		if sa == 0 {
			t.Fatalf("light=%v: разделитель прозрачный", light)
		}
		// линия должна контрастировать с фоном: светлая на тёмном и наоборот
		bg := th.Color(theme.ColorNameBackground, variant)
		bgr, bgg, bgb, _ := bg.RGBA()
		if !light && (sr <= bgr && sg <= bgg && sb <= bgb) {
			t.Errorf("тёмная тема: разделитель не светлее фона")
		}
		if light && (sr >= bgr && sg >= bgg && sb >= bgb) {
			t.Errorf("светлая тема: разделитель не темнее фона")
		}

		// остальные цвета должны приходить из базовой темы
		want := theme.DarkTheme().Color(theme.ColorNameForeground, variant)
		if light {
			want = theme.LightTheme().Color(theme.ColorNameForeground, variant)
		}
		if got := th.Color(theme.ColorNameForeground, variant); got != want {
			t.Errorf("light=%v: цвет текста подменён", light)
		}
	}
}

// Текст статуса должен влезать в строку заголовка на узком экране.
func TestStatusTextFitsPhoneWidth(t *testing.T) {
	test.NewApp()

	// на телефоне (536 px при DPR ~2.75) логическая ширина ~195,
	// из них кнопка настроек + отступы забирают ~48
	const available float32 = 145

	// тексты строки статуса (all_good — это текст в центре экрана, он может
	// быть длиннее; длинные ошибки обрезаются уже в самой метке)
	texts := []string{
		fmt.Sprintf(data.Tr("problems_count"), 12),
		fmt.Sprintf(data.Tr("problems_count"), 999),
		data.Tr("configure_server"),
		data.Tr("waiting_data"),
		data.Tr("api_error"),
		fmt.Sprintf(data.Tr("cached_data"), "21.10 14:33"),
	}

	for _, lang := range data.KnownLangs() {
		data.SetLang(lang)
		for _, text := range texts {
			width := fyne.MeasureText(text, theme.TextSize(), fyne.TextStyle{Bold: true}).Width
			if width > available {
				t.Errorf("язык %s: статус %q не влезает (%.0f > %.0f)", lang, text, width, available)
			}
		}
	}
	data.SetLang("ru")
}

// Разделители должны быть видны и на отрендеренном экране, а не только в
// цвете темы: в базовой тёмной теме fyne линия получается чёрной на тёмном фоне
// (яркость 0), с нашей темой — светлая линия в 1 px между карточками.
func TestListSeparatorsAreVisible(t *testing.T) {
	newApp := test.NewApp()
	newApp.Settings().SetTheme(appTheme(false))

	problems := []data.Problem{
		{EventID: "1", Clock: "1700000000", Severity: "3", HostName: "host-a", Name: "ICMP down"},
		{EventID: "2", Clock: "1700000060", Severity: "5", HostName: "host-b", Name: "Signal low"},
		{EventID: "3", Clock: "1700000120", Severity: "1", HostName: "host-c", Name: "CPU high"},
	}

	list := widget.NewList(
		func() int { return len(problems) },
		func() fyne.CanvasObject { return newProblemRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*problemRow).setProblem(problems[i]) },
	)

	win := test.NewWindow(list)
	defer win.Close()
	win.Resize(fyne.NewSize(300, 300))
	list.Refresh()
	win.Resize(fyne.NewSize(300, 300))
	list.Refresh()

	img := win.Canvas().Capture()
	bounds := img.Bounds()

	// строка-разделитель однородна по всей ширине: минимальная яркость в ней
	// заметно выше фона (у текста минимум = фон).
	lines := 0
	for y := 0; y < bounds.Dy(); y++ {
		minBrightness := uint32(0xffff)
		for x := 20; x < bounds.Dx()-20; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			value := (r >> 8) + (g >> 8) + (bl >> 8)
			if value < minBrightness {
				minBrightness = value
			}
		}
		if float32(minBrightness)/3 > 40 {
			lines++
		}
	}

	if lines == 0 {
		t.Fatal("между карточками нет видимой линии-разделителя")
	}
	if lines > 4 {
		t.Errorf("разделители слишком толстые: %d строк", lines)
	}
}

func TestMiddleEllipsis(t *testing.T) {
	runes := []rune("abcdefghijklmnop")

	if got := middleEllipsis(runes, len(runes)); got != "abcdefghijklmnop" {
		t.Errorf("middleEllipsis(full) = %q", got)
	}
	if got := middleEllipsis(runes, 1); got != "…" {
		t.Errorf("middleEllipsis(1) = %q", got)
	}

	// 7 исходных символов: 2 из начала (40%) + «…» + 5 из конца (60%)
	if got := middleEllipsis(runes, 7); got != "ab…lmnop" {
		t.Errorf("middleEllipsis(7) = %q, want %q", got, "ab…lmnop")
	}
	// хвост всегда длиннее начала: значимая часть имени хоста обычно в конце
	head, tail, ok := splitEllipsis(middleEllipsis(runes, 10))
	if !ok || len([]rune(tail)) <= len([]rune(head)) {
		t.Errorf("middleEllipsis(10) = %q, хвост должен быть длиннее начала", middleEllipsis(runes, 10))
	}
}
