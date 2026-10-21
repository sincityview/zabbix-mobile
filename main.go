package main

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"zabbix/data"
)

const appID = "com.zabbix.android.monitor"

type monitor struct {
	app    fyne.App
	store  *data.Store
	snap   *data.SnapshotStore
	window fyne.Window

	settingsWin fyne.Window

	status     binding.String
	statusLbl  *widget.Label
	list       *widget.List
	welcome    *widget.RichText
	welcomeBox *fyne.Container
	mainStack  *fyne.Container
	refreshBtn *widget.Button

	mu sync.Mutex
	// settingsSource — откуда загружены настройки (для диагностики).
	settingsSource string
	settings       data.Settings
	client         *data.Client
	snapKey        string
	problems       []data.Problem
	loading        bool
	// gen меняется при смене настроек: устаревший ответ не должен затирать вид.
	gen uint64

	intervalCh chan time.Duration
	pauseCh    chan bool
}

func main() {
	a := app.NewWithID(appID)
	m := newMonitor(a)
	m.run()
}

func newMonitor(a fyne.App) *monitor {
	m := &monitor{
		app:        a,
		store:      data.NewStore(),
		snap:       data.NewSnapshotStore(),
		intervalCh: make(chan time.Duration, 1),
		pauseCh:    make(chan bool, 1),
	}

	m.settings = m.loadSettings()

	m.status = binding.NewString()
	m.statusLbl = widget.NewLabelWithData(m.status)
	m.statusLbl.TextStyle = fyne.TextStyle{Bold: true}
	m.statusLbl.Truncation = fyne.TextTruncateEllipsis

	m.list = widget.NewList(
		func() int {
			m.mu.Lock()
			defer m.mu.Unlock()
			return len(m.problems)
		},
		func() fyne.CanvasObject {
			return newProblemRow()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			row, ok := obj.(*problemRow)
			if !ok {
				return
			}
			if p, ok := m.problemAt(id); ok {
				row.setProblem(p)
				return
			}
			row.clear()
		},
	)
	// Тап по карточке открывает подробности целиком: в строке текст обрезан,
	// чтобы высота карточки не зависела от длины названий.
	m.list.OnSelected = func(id widget.ListItemID) {
		m.list.Unselect(id)
		if p, ok := m.problemAt(id); ok {
			m.showProblemDetails(p)
		}
	}

	m.welcome = widget.NewRichText(&widget.TextSegment{
		Text:  data.Tr("waiting_data"),
		Style: welcomeStyle,
	})
	m.welcomeBox = container.NewCenter(m.welcome)
	m.mainStack = container.NewStack(m.welcomeBox, m.list)

	// Сразу показываем последний удачный ответ: холодный старт с сетевым
	// запросом выглядит на телефоне как пустое приложение.
	m.snapKey = data.SnapshotKey(m.settings.ClientConfig())
	if problems, savedAt, ok := m.snap.Load(m.snapKey, time.Now()); ok {
		m.problems = problems
		_ = m.status.Set(fmt.Sprintf(data.Tr("cached_data"), savedAt.Local().Format("02.01 15:04")))
		m.list.Refresh()
		m.welcomeBox.Hide()
		m.list.Show()
	} else {
		_ = m.status.Set(data.Tr("waiting_data"))
	}

	return m
}

func (m *monitor) run() {
	m.applySettings(m.settings)

	m.window = m.app.NewWindow(data.Tr("app_title"))
	m.window.Resize(fyne.NewSize(450, 650))
	m.window.SetContent(m.buildContent())

	m.registerLifecycle()
	go m.autoRefresh()

	m.refresh()
	m.window.ShowAndRun()
}

func (m *monitor) buildContent() fyne.CanvasObject {
	settingsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), m.openSettings)

	m.refreshBtn = widget.NewButtonWithIcon(data.Tr("update"), theme.ViewRefreshIcon(), m.refresh)

	// Border, а не HBox: у label с Truncation MinSize равен ширине одного
	// символа (widget/richtext.go, calculateMin), поэтому в HBox статус
	// сжимался в «…». В Border центральный элемент получает всю оставшуюся
	// ширину, и текст рисуется целиком (обрезаясь только если реально не влез).
	topBar := container.NewBorder(nil, nil, nil, settingsBtn, m.statusLbl)

	return container.NewBorder(
		container.NewVBox(topBar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), m.refreshBtn),
		nil, nil,
		m.mainStack,
	)
}

// registerLifecycle останавливает опрос сервера, когда приложение уходит в фон:
// на телефоне это экономит батарею и трафик, а при возврате данные обновляются
// сразу.
func (m *monitor) registerLifecycle() {
	lc := m.app.Lifecycle()
	lc.SetOnExitedForeground(func() { m.setPaused(true) })
	lc.SetOnEnteredForeground(func() {
		m.setPaused(false)
		go m.refresh()
	})
}

func (m *monitor) setPaused(paused bool) {
	select {
	case <-m.pauseCh:
	default:
	}
	select {
	case m.pauseCh <- paused:
	default:
	}
}

func (m *monitor) notifyInterval(d time.Duration) {
	select {
	case <-m.intervalCh:
	default:
	}
	select {
	case m.intervalCh <- d:
	default:
	}
}

func (m *monitor) interval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings.Interval()
}

// autoRefresh — один таймер, который перезапускается при смене интервала и
// встаёт на паузу в фоне. В отличие от sleep-цикла, новый интервал применяется
// сразу, а не после текущего сна.
func (m *monitor) autoRefresh() {
	timer := time.NewTimer(m.interval())
	defer timer.Stop()

	paused := false
	for {
		select {
		case d := <-m.intervalCh:
			if !paused {
				resetTimer(timer, d)
			}
		case p := <-m.pauseCh:
			paused = p
			if paused {
				stopTimer(timer)
				continue
			}
			resetTimer(timer, m.interval())
			go m.refresh()
		case <-timer.C:
			if paused {
				continue
			}
			go m.refresh()
			timer.Reset(m.interval())
		}
	}
}

func resetTimer(t *time.Timer, d time.Duration) {
	if d <= 0 {
		d = data.DefaultRefreshSeconds * time.Second
	}
	stopTimer(t)
	t.Reset(d)
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// refresh можно вызывать из любой горутины.
func (m *monitor) refresh() {
	m.mu.Lock()
	if m.loading {
		m.mu.Unlock()
		return
	}
	st, client, gen, key := m.settings, m.client, m.gen, m.snapKey
	if client == nil || st.URL == "" || !st.HasCredentials() {
		m.mu.Unlock()
		fyne.Do(func() {
			_ = m.status.Set(data.Tr("configure_server"))
			m.showWelcomeUI(data.Tr("waiting_data"))
		})
		return
	}
	m.loading = true
	m.mu.Unlock()

	go func() {
		problems, err := client.Problems()
		fyne.Do(func() {
			m.applyResult(problems, err)

			// Пока шёл запрос, пользователь мог поменять настройки — тогда
			// повторяем с новым клиентом.
			m.mu.Lock()
			stale := m.gen != gen
			m.mu.Unlock()
			if stale {
				go m.refresh()
			}
		})

		// Снапшот пишем вне UI-потока и только при успешном ответе.
		if err == nil {
			if saveErr := m.snap.Save(key, problems); saveErr != nil {
				fyne.LogError("snapshot: сохранение", saveErr)
			}
		}
	}()
}

// applyResult вызывается в UI-потоке.
func (m *monitor) applyResult(problems []data.Problem, err error) {
	var hostErr *data.HostLookupError
	if err != nil && !errors.As(err, &hostErr) {
		m.mu.Lock()
		m.loading = false
		m.mu.Unlock()
		_ = m.status.Set(fmt.Sprintf("%s: %s", data.Tr("api_error"), shortError(err)))
		return
	}

	m.mu.Lock()
	m.loading = false
	prev := m.problems
	changed := !problemsEqual(prev, problems)
	m.problems = problems
	m.mu.Unlock()

	// Если данные не изменились — не перерисовываем список: полный Refresh
	// видимых строк на телефоне заметно дороже, чем сравнение.
	if changed {
		m.list.Refresh()
	}

	count := fmt.Sprintf(data.Tr("problems_count"), len(problems))

	switch {
	case err != nil:
		_ = m.status.Set(fmt.Sprintf("%s (%s)", count, data.Tr("no_host_names")))
		m.showListUI()
	case len(problems) == 0:
		_ = m.status.Set(count)
		m.showWelcomeUI(data.Tr("all_good"))
	default:
		_ = m.status.Set(count)
		m.showListUI()
	}
}

func (m *monitor) showWelcomeUI(text string) {
	m.welcome.Segments = []widget.RichTextSegment{&widget.TextSegment{Text: text, Style: welcomeStyle}}
	m.welcome.Refresh()
	m.list.Hide()
	m.welcomeBox.Show()
}

func (m *monitor) showListUI() {
	m.welcomeBox.Hide()
	m.list.Show()
}

func (m *monitor) problemAt(id widget.ListItemID) (data.Problem, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id < 0 || int(id) >= len(m.problems) {
		return data.Problem{}, false
	}
	return m.problems[id], true
}

func (m *monitor) currentSettings() data.Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

// applySettings применяет настройки к UI и пересоздаёт API-клиент.
// Вызывается из UI-потока.
func (m *monitor) applySettings(st data.Settings) {
	st = st.Normalized()
	key := data.SnapshotKey(st.ClientConfig())

	m.mu.Lock()
	prevKey := m.snapKey
	m.settings = st
	m.client = data.NewClient(st.ClientConfig())
	m.snapKey = key
	m.gen++
	// Данные другого сервера или токена показывать нельзя.
	keyChanged := prevKey != "" && prevKey != key
	if keyChanged {
		m.problems = nil
	}
	m.mu.Unlock()

	if keyChanged {
		m.list.Refresh()
		m.showWelcomeUI(data.Tr("waiting_data"))
	}

	data.SetLang(st.Lang)
	m.app.Settings().SetTheme(appTheme(st.Theme == "light"))

	if m.window != nil {
		m.window.SetTitle(data.Tr("app_title"))
	}
	if m.refreshBtn != nil {
		m.refreshBtn.SetText(data.Tr("update"))
	}
	m.notifyInterval(st.Interval())

	// Тема/язык могли поменяться — обновляем открытое окно настроек.
	if m.settingsWin != nil {
		if content := m.settingsWin.Content(); content != nil {
			content.Refresh()
		}
	}
}

// resetSettings удаляет с устройства сохранённые адрес, токен, пароль и
// снапшот проблем, возвращая приложение в исходное состояние.
func (m *monitor) resetSettings() error {
	var errs []error
	if err := m.store.Clear(); err != nil {
		errs = append(errs, err)
	}
	m.snap.Clear()
	m.clearPrefs()

	m.settingsSource = ""

	m.mu.Lock()
	m.problems = nil
	m.mu.Unlock()

	m.list.Refresh()
	m.applySettings(data.DefaultSettings())
	m.showWelcomeUI(data.Tr("configure_server"))
	_ = m.status.Set(data.Tr("configure_server"))

	return errors.Join(errs...)
}

// separatorTheme делает разделители видимыми.
//
// В тёмной теме fyne рисует их чисто чёрным (theme.colorDarkSeparator =
// {0,0,0,255}) на почти чёрном фоне — линия не видна вообще, поэтому карточки
// в списке сливались. Подменяем только цвет разделителя, остальное берём из
// базовой темы.
type separatorTheme struct {
	fyne.Theme
}

// appTheme собирает тему приложения: базовая тема + видимые разделители.
func appTheme(light bool) fyne.Theme {
	base := fyne.Theme(theme.DarkTheme())
	if light {
		base = theme.LightTheme()
	}
	return separatorTheme{Theme: base}
}

func (t separatorTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameSeparator {
		if variant == theme.VariantLight {
			// полупрозрачный чёрный поверх светлого фона
			return color.NRGBA{A: 0x38}
		}
		// полупрозрачный белый поверх тёмного фона
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x50}
	}
	return t.Theme.Color(name, variant)
}

var welcomeStyle = widget.RichTextStyle{
	Alignment: fyne.TextAlignCenter,
	TextStyle: fyne.TextStyle{Bold: true},
}

// severityColor — цвет полосы важности (0..5, как в Zabbix).
func severityColor(level int) color.Color {
	switch level {
	case 5: // Disaster
		return color.RGBA{R: 220, G: 0, B: 0, A: 255}
	case 4: // High
		return color.RGBA{R: 255, G: 153, B: 0, A: 255}
	case 3: // Average
		return color.RGBA{R: 255, G: 255, B: 0, A: 255}
	case 2: // Warning
		return color.RGBA{R: 255, G: 200, B: 100, A: 255}
	case 1: // Information
		return color.RGBA{R: 100, G: 150, B: 255, A: 255}
	default:
		return theme.Color(theme.ColorNameForeground)
	}
}

// rowBarWidth — ширина цветной полосы важности.
const rowBarWidth float32 = 6

// problemRow — строка списка как отдельный виджет.
//
// Каждое поле — ровно одна строка с обрезкой: у widget.List высота элемента
// фиксированная (одна на все строки, см. l.itemMin в widget/list.go), поэтому
// перенос длинного имени хоста на вторую строку выталкивал текст проблемы за
// границу карточки. Длинные значения обрезаются посередине (чтобы сохранить
// хвост вида "[ ELTEX MES2324FB ]"), а полностью текст виден по тапу.
type problemRow struct {
	widget.BaseWidget

	bar       *canvas.Rectangle
	timeLabel *widget.Label
	hostLabel *widget.Label
	nameLabel *widget.Label

	full     data.Problem
	filled   bool
	fitWidth float32
}

func newProblemRow() *problemRow {
	r := &problemRow{
		bar:       canvas.NewRectangle(color.White),
		timeLabel: widget.NewLabel(""),
		hostLabel: widget.NewLabel(""),
		nameLabel: widget.NewLabel(""),
	}

	r.bar.SetMinSize(fyne.NewSize(rowBarWidth, 0))
	r.timeLabel.TextStyle = fyne.TextStyle{Italic: true}
	r.hostLabel.TextStyle = fyne.TextStyle{Bold: true}

	for _, label := range []*widget.Label{r.timeLabel, r.hostLabel, r.nameLabel} {
		label.Wrapping = fyne.TextWrapOff
		label.Truncation = fyne.TextTruncateEllipsis
	}

	r.ExtendBaseWidget(r)
	return r
}

// setProblem обновляет строку, только если её данные действительно изменились.
func (r *problemRow) setProblem(p data.Problem) {
	if r.filled && r.full == p {
		return
	}

	r.full = p
	r.filled = true
	r.fitWidth = 0 // текст мог измениться — пересчитать под ширину

	r.timeLabel.SetText(data.FormatTime(p.Clock))
	r.bar.FillColor = severityColor(p.SeverityLevel())
	r.bar.Refresh()
	r.refit()
}

func (r *problemRow) clear() {
	r.full = data.Problem{}
	r.filled = false
	r.fitWidth = 0
	r.timeLabel.SetText("")
	r.hostLabel.SetText("")
	r.nameLabel.SetText("")
}

// Resize вызывается списком при изменении ширины — пересчитываем обрезку.
func (r *problemRow) Resize(size fyne.Size) {
	r.BaseWidget.Resize(size)
	r.refit()
}

// refit подгоняет хост и текст проблемы под доступную ширину карточки.
func (r *problemRow) refit() {
	if !r.filled {
		return
	}
	width := r.Size().Width - rowBarWidth - 2*theme.Padding()
	if width <= 0 || width == r.fitWidth {
		return
	}
	r.fitWidth = width

	size := theme.TextSize()
	r.hostLabel.SetText(fitMiddle(r.full.HostName, width, size, r.hostLabel.TextStyle))
	r.nameLabel.SetText(fitMiddle(r.full.Name, width, size, r.nameLabel.TextStyle))
}

func (r *problemRow) CreateRenderer() fyne.WidgetRenderer {
	body := container.NewPadded(container.NewVBox(r.timeLabel, r.hostLabel, r.nameLabel))
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, r.bar, nil, body))
}

// fitMiddle обрезает текст посередине так, чтобы он уложился в width.
func fitMiddle(text string, width, size float32, style fyne.TextStyle) string {
	if text == "" {
		return ""
	}
	if fyne.MeasureText(text, size, style).Width <= width {
		return text
	}

	runes := []rune(text)
	if len(runes) < 3 {
		return "…"
	}

	best := "…"
	lo, hi := 1, len(runes)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		candidate := middleEllipsis(runes, mid)
		if fyne.MeasureText(candidate, size, style).Width <= width {
			best = candidate
			lo = mid + 1
			continue
		}
		hi = mid - 1
	}
	return best
}

// middleEllipsis оставляет keep исходных символов, обрезая середину.
//
// Хвосту отдаётся 60%: в именах хостов Zabbix значимая часть обычно в конце
// («... [ ELTEX MES2324FB ]»), а начало (улица, дом) и так узнаваемо.
func middleEllipsis(runes []rune, keep int) string {
	if keep >= len(runes) {
		return string(runes)
	}
	if keep <= 1 {
		return "…"
	}

	head := keep * 2 / 5
	if head < 1 {
		head = 1
	}
	tail := keep - head
	if tail <= 0 {
		return string(runes[:head]) + "…"
	}
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

// showProblemDetails показывает карточку целиком (текст в строке обрезан).
func (m *monitor) showProblemDetails(p data.Problem) {
	line := func(format string, args ...any) *widget.Label {
		l := widget.NewLabel(fmt.Sprintf(format, args...))
		l.Wrapping = fyne.TextWrapWord
		return l
	}

	acknowledged := data.Tr("no")
	if p.AcknowledgedProblem() {
		acknowledged = data.Tr("yes")
	}

	content := container.NewVBox(
		line("%s: %s", data.Tr("time"), data.FormatTime(p.Clock)),
		line("%s: %s", data.Tr("host"), p.HostName),
		line("%s: %s", data.Tr("problem"), p.Name),
		line("%s: %s", data.Tr("severity"), severityName(p.SeverityLevel())),
		line("%s: %s", data.Tr("acknowledged"), acknowledged),
	)

	scroll := container.NewVScroll(container.NewPadded(content))
	scroll.SetMinSize(fyne.NewSize(300, 180))

	dialog.NewCustom(data.Tr("problem_details"), data.Tr("close"), scroll, m.window).Show()
}

// severityName переводит уровень важности в человекочитаемое имя.
func severityName(level int) string {
	switch level {
	case 5:
		return data.Tr("severity_5")
	case 4:
		return data.Tr("severity_4")
	case 3:
		return data.Tr("severity_3")
	case 2:
		return data.Tr("severity_2")
	case 1:
		return data.Tr("severity_1")
	case 0:
		return data.Tr("severity_0")
	default:
		return data.Tr("severity_unknown")
	}
}

// problemsEqual сравнивает два ответа API без аллокаций.
func problemsEqual(a, b []data.Problem) bool {
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

// shortError укорачивает текст ошибки для строки статуса.
func shortError(err error) string {
	s := strings.Join(strings.Fields(strings.ReplaceAll(err.Error(), "\n", " ")), " ")
	runes := []rune(s)
	if len(runes) > 70 {
		return string(runes[:70]) + "…"
	}
	return s
}
