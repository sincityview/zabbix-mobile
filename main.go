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
	"fyne.io/fyne/v2/layout"
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

	topBar := container.NewHBox(m.statusLbl, layout.NewSpacer(), settingsBtn)

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
	if fresh := countNew(prev, problems); fresh > 0 {
		count += " · " + fmt.Sprintf(data.Tr("new_problems"), fresh)
	}

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
	if st.Theme == "light" {
		m.app.Settings().SetTheme(theme.LightTheme())
	} else {
		m.app.Settings().SetTheme(theme.DarkTheme())
	}

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

// problemRow — строка списка как отдельный виджет: типизированные поля вместо
// обхода дерева объектов с приведением типов при каждом обновлении.
type problemRow struct {
	widget.BaseWidget

	bar       *canvas.Rectangle
	timeLabel *widget.Label
	hostLabel *widget.Label
	nameLabel *widget.Label

	lastEventID, lastName, lastHost, lastClock, lastSeverity string
}

func newProblemRow() *problemRow {
	r := &problemRow{
		bar:       canvas.NewRectangle(color.White),
		timeLabel: widget.NewLabel(""),
		hostLabel: widget.NewLabel(""),
		nameLabel: widget.NewLabel(""),
	}

	r.bar.SetMinSize(fyne.NewSize(6, 0))
	r.timeLabel.TextStyle = fyne.TextStyle{Italic: true}
	r.hostLabel.TextStyle = fyne.TextStyle{Bold: true}
	r.hostLabel.Wrapping = fyne.TextWrapWord
	r.nameLabel.Wrapping = fyne.TextWrapWord

	r.ExtendBaseWidget(r)
	return r
}

// setProblem обновляет строку, только если её данные действительно изменились.
func (r *problemRow) setProblem(p data.Problem) {
	if r.lastEventID == p.EventID && r.lastName == p.Name && r.lastHost == p.HostName &&
		r.lastClock == p.Clock && r.lastSeverity == p.Severity {
		return
	}
	r.lastEventID, r.lastName, r.lastHost = p.EventID, p.Name, p.HostName
	r.lastClock, r.lastSeverity = p.Clock, p.Severity

	r.timeLabel.SetText(data.FormatTime(p.Clock))
	r.hostLabel.SetText(p.HostName)
	r.nameLabel.SetText(p.Name)
	r.bar.FillColor = severityColor(p.SeverityLevel())
	r.bar.Refresh()
}

func (r *problemRow) clear() {
	r.lastEventID, r.lastName, r.lastHost, r.lastClock, r.lastSeverity = "", "", "", "", ""
	r.timeLabel.SetText("")
	r.hostLabel.SetText("")
	r.nameLabel.SetText("")
}

func (r *problemRow) CreateRenderer() fyne.WidgetRenderer {
	body := container.NewPadded(container.NewVBox(r.timeLabel, r.hostLabel, r.nameLabel))
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, r.bar, nil, body))
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

// countNew считает проблемы, которых не было в предыдущем ответе.
func countNew(prev, next []data.Problem) int {
	if len(prev) == 0 || len(next) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(prev))
	for i := range prev {
		seen[prev[i].EventID] = struct{}{}
	}
	fresh := 0
	for i := range next {
		if _, ok := seen[next[i].EventID]; !ok {
			fresh++
		}
	}
	return fresh
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
