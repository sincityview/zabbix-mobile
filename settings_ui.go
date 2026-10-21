package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"zabbix/data"
)

// Ключи в fyne.Preferences: нужны для переноса настроек из старых версий и как
// зеркало (чтобы данные не потерялись при откате на старый APK).
const (
	prefURL        = "ZABBIX_URL"
	prefToken      = "ZABBIX_TOKEN"
	prefUser       = "ZABBIX_USER"
	prefPass       = "ZABBIX_PASS"
	prefSelfSigned = "SELF_SIGNED"
	prefInterval   = "REFRESH_INTERVAL"
	prefLimit      = "PROBLEM_LIMIT"
	prefTheme      = "THEME"
	prefLang       = "LANG"
)

// errInvalidNumber — поле должно быть положительным числом.
var errInvalidNumber = errors.New("invalid number")

// loadSettings читает настройки из надёжного хранилища. Если его нет — переносит
// значения из fyne.Preferences (старые версии) и сразу сохраняет их.
func (m *monitor) loadSettings() data.Settings {
	st, source, err := m.store.LoadWithSource()
	if err == nil {
		m.settingsSource = source
		if source == data.SettingsBackupFile {
			fyne.LogError("settings: основной файл повреждён, настройки взяты из резервной копии", nil)
		}
		return st
	}
	if !errors.Is(err, data.ErrNoSettings) {
		fyne.LogError("settings: чтение настроек", err)
	}

	legacy := m.settingsFromPrefs()
	if legacy.URL != "" || legacy.HasCredentials() {
		if saveErr := m.store.Save(legacy); saveErr != nil {
			fyne.LogError("settings: перенос настроек", saveErr)
		}
		m.settingsSource = data.SettingsMainFile
		return legacy
	}

	def := data.DefaultSettings()
	def.Theme = legacy.Theme
	def.Lang = legacy.Lang
	return def
}

func (m *monitor) settingsFromPrefs() data.Settings {
	prefs := m.app.Preferences()
	st := data.DefaultSettings()

	st.URL = prefs.String(prefURL)
	st.Token = data.Secret(prefs.String(prefToken))
	st.User = prefs.String(prefUser)
	st.Password = data.Secret(prefs.String(prefPass))
	st.SelfSigned = prefs.BoolWithFallback(prefSelfSigned, false)

	if v, err := strconv.Atoi(prefs.StringWithFallback(prefInterval, "")); err == nil && v > 0 {
		st.RefreshSeconds = v
	}
	if v, err := strconv.Atoi(prefs.StringWithFallback(prefLimit, "")); err == nil && v > 0 {
		st.Limit = v
	}
	if theme := prefs.String(prefTheme); theme != "" {
		st.Theme = theme
	}
	if lang := prefs.String(prefLang); lang != "" {
		st.Lang = lang
	}
	return st
}

// mirrorToPrefs пишет настройки в fyne.Preferences как резервную копию.
func (m *monitor) mirrorToPrefs(st data.Settings) {
	prefs := m.app.Preferences()
	prefs.SetString(prefURL, st.URL)
	prefs.SetString(prefToken, st.Token.Value())
	prefs.SetString(prefUser, st.User)
	prefs.SetString(prefPass, st.Password.Value())
	prefs.SetBool(prefSelfSigned, st.SelfSigned)
	prefs.SetString(prefInterval, strconv.Itoa(st.RefreshSeconds))
	prefs.SetString(prefLimit, strconv.Itoa(st.Limit))
	prefs.SetString(prefTheme, st.Theme)
	prefs.SetString(prefLang, st.Lang)
}

// clearPrefs удаляет зеркало настроек из fyne.Preferences (при сбросе).
func (m *monitor) clearPrefs() {
	prefs := m.app.Preferences()
	for _, key := range []string{
		prefURL, prefToken, prefUser, prefPass, prefSelfSigned,
		prefInterval, prefLimit, prefTheme, prefLang,
	} {
		prefs.RemoveValue(key)
	}
}

// openSettings открывает окно настроек. Вызывается из UI-потока.
func (m *monitor) openSettings() {
	if m.settingsWin != nil {
		m.settingsWin.RequestFocus()
		return
	}

	cur := m.currentSettings()

	win := m.app.NewWindow(data.Tr("settings"))
	win.Resize(fyne.NewSize(440, 680))
	win.CenterOnScreen()
	m.settingsWin = win
	win.SetOnClosed(func() { m.settingsWin = nil })

	urlEntry := widget.NewEntry()
	urlEntry.SetText(cur.URL)
	urlEntry.SetPlaceHolder("https://zabbix.example.com/api_jsonrpc.php")
	urlEntry.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil // пустое поле проверяется при сохранении
		}
		if err := data.ValidateURL(s); err != nil {
			return errors.New(data.Tr("error_url_invalid"))
		}
		return nil
	}

	tokenEntry := widget.NewPasswordEntry()
	tokenEntry.SetText(cur.Token.Value())

	userEntry := widget.NewEntry()
	userEntry.SetText(cur.User)

	passEntry := widget.NewPasswordEntry()
	passEntry.SetText(cur.Password.Value())

	selfSignedCheck := widget.NewCheck(data.Tr("self_signed"), nil)
	selfSignedCheck.SetChecked(cur.SelfSigned)

	intervalEntry := widget.NewEntry()
	intervalEntry.SetText(strconv.Itoa(cur.RefreshSeconds))

	limitEntry := widget.NewEntry()
	limitEntry.SetText(strconv.Itoa(cur.Limit))

	themeSelect := widget.NewSelect([]string{"Dark", "Light"}, nil)
	if cur.Theme == "light" {
		themeSelect.SetSelected("Light")
	} else {
		themeSelect.SetSelected("Dark")
	}

	langSelect := widget.NewSelect([]string{"Русский", "English"}, nil)
	if cur.Lang == "en" {
		langSelect.SetSelected("English")
	} else {
		langSelect.SetSelected("Русский")
	}

	errorLabel := widget.NewLabel("")
	errorLabel.TextStyle = fyne.TextStyle{Bold: true}
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Hide()

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	// Предупреждение о незашифрованном подключении: токен ушёл бы по сети
	// открытым текстом.
	cleartextLabel := widget.NewLabel(data.Tr("cleartext_warning"))
	cleartextLabel.Wrapping = fyne.TextWrapWord
	cleartextLabel.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
	cleartextLabel.SizeName = theme.SizeNameCaptionText

	updateCleartext := func(raw string) {
		cfg := data.Config{URL: raw}
		if cfg.Cleartext() {
			cleartextLabel.Show()
			return
		}
		cleartextLabel.Hide()
	}
	urlEntry.OnChanged = func(s string) { updateCleartext(s) }

	showError := func(msg string) {
		errorLabel.SetText(msg)
		errorLabel.Show()
	}

	// collect собирает и проверяет значения формы.
	collect := func() (data.Settings, error) {
		st := data.Settings{
			URL:            urlEntry.Text,
			Token:          data.Secret(tokenEntry.Text),
			User:           userEntry.Text,
			Password:       data.Secret(passEntry.Text),
			SelfSigned:     selfSignedCheck.Checked,
			Theme:          "dark",
			Lang:           "ru",
			RefreshSeconds: cur.RefreshSeconds,
			Limit:          cur.Limit,
		}
		if themeSelect.Selected == "Light" {
			st.Theme = "light"
		}
		if langSelect.Selected == "English" {
			st.Lang = "en"
		}

		interval, err := positiveInt(intervalEntry.Text, cur.RefreshSeconds)
		if err != nil {
			return st, err
		}
		limit, err := positiveInt(limitEntry.Text, cur.Limit)
		if err != nil {
			return st, err
		}
		st.RefreshSeconds, st.Limit = interval, limit

		st = st.Normalized()
		if err := st.Validate(); err != nil {
			return st, err
		}
		return st, nil
	}

	var testBtn *widget.Button
	testBtn = widget.NewButton(data.Tr("test_connection"), func() {
		errorLabel.Hide()
		st, err := collect()
		if err != nil {
			showError(settingsErrorText(err))
			return
		}

		testBtn.Disable()
		statusLabel.SetText(data.Tr("waiting_data"))

		go func() {
			problems, err := data.NewClient(st.ClientConfig()).Problems()
			fyne.Do(func() {
				testBtn.Enable()
				if err != nil {
					statusLabel.SetText(fmt.Sprintf(data.Tr("connection_failed"), shortError(err)))
					return
				}
				statusLabel.SetText(fmt.Sprintf(data.Tr("connection_ok"), len(problems)))
			})
		}()
	})

	resetBtn := widget.NewButton(data.Tr("reset_settings"), func() {
		dialog.ShowConfirm(data.Tr("reset_settings"), data.Tr("reset_confirm"), func(ok bool) {
			if !ok {
				return
			}
			if err := m.resetSettings(); err != nil {
				showError(fmt.Sprintf(data.Tr("error_save_settings"), shortError(err)))
				return
			}
			win.Close()
		}, win)
	})
	resetBtn.Importance = widget.DangerImportance

	form := container.NewVBox(
		widget.NewLabel(data.Tr("url_server")),
		urlEntry,
		cleartextLabel,
		widget.NewLabel(data.Tr("token")),
		tokenEntry,
		widget.NewLabel(data.Tr("username")),
		userEntry,
		widget.NewLabel(data.Tr("password")),
		passEntry,
		selfSignedCheck,
		widget.NewLabel(data.Tr("refresh_interval")),
		intervalEntry,
		widget.NewLabel(data.Tr("problem_limit")),
		limitEntry,
		widget.NewLabel(data.Tr("theme")),
		themeSelect,
		widget.NewLabel(data.Tr("language")),
		langSelect,
		widget.NewSeparator(),
		testBtn,
		statusLabel,
		errorLabel,
		widget.NewSeparator(),
		resetBtn,
	)

	cancelBtn := widget.NewButton(data.Tr("cancel"), func() { win.Close() })

	saveBtn := widget.NewButton(data.Tr("save"), func() {
		errorLabel.Hide()

		st, err := collect()
		if err != nil {
			showError(settingsErrorText(err))
			return
		}

		// Сначала атомарная запись в надёжное хранилище: если она не удалась,
		// сообщаем об этом, а не закрываем окно молча.
		if err := m.store.Save(st); err != nil {
			showError(fmt.Sprintf(data.Tr("error_save_settings"), shortError(err)))
			return
		}
		m.settingsSource = data.SettingsMainFile
		m.mirrorToPrefs(st)
		m.applySettings(st)

		win.Close()
		go m.refresh()
	})

	buttons := container.NewGridWithColumns(2, cancelBtn, saveBtn)

	// Подсказка для диагностики: где лежат настройки и не восстановлены ли они
	// из резервной копии.
	infoText := fmt.Sprintf(data.Tr("settings_file"), m.store.Dir())
	if m.settingsSource == data.SettingsBackupFile {
		infoText += " " + data.Tr("settings_restored")
	}
	infoLabel := widget.NewLabel(infoText)
	infoLabel.Wrapping = fyne.TextWrapWord
	infoLabel.TextStyle = fyne.TextStyle{Italic: true}
	infoLabel.SizeName = theme.SizeNameCaptionText

	bottom := container.NewVBox(widget.NewSeparator(), buttons, infoLabel)

	// Прокрутка обязательна: на телефоне форма не помещается в экран.
	content := container.NewBorder(nil, bottom, nil, nil, container.NewVScroll(form))

	updateCleartext(cur.URL)
	win.SetContent(container.NewPadded(content))
	win.Show()
}

// settingsErrorText переводит ошибку валидации в текст интерфейса.
func settingsErrorText(err error) string {
	switch {
	case errors.Is(err, data.ErrURLRequired):
		return data.Tr("error_url_required")
	case errors.Is(err, data.ErrURLInvalid):
		return data.Tr("error_url_invalid")
	case errors.Is(err, data.ErrCredentialsRequired):
		return data.Tr("error_credentials_required")
	case errors.Is(err, errInvalidNumber):
		return data.Tr("error_invalid_number")
	default:
		return data.Tr("error_invalid_number")
	}
}

// positiveInt читает положительное целое; пустая строка — значение по умолчанию.
func positiveInt(text string, def int) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return def, nil
	}
	v, err := strconv.Atoi(text)
	if err != nil || v <= 0 {
		return def, errInvalidNumber
	}
	return v, nil
}
