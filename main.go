package main

import (
	"fmt"
	"strconv"
	"time"

	"image/color"

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

var (
	problemsData []data.Problem
	refreshBtn   *widget.Button
)

func getSeverityColor(severity string) color.Color {
	switch severity {
	case "5":
		return color.RGBA{R: 220, G: 0, B: 0, A: 255}
	case "4":
		return color.RGBA{R: 255, G: 153, B: 0, A: 255}
	case "3":
		return color.RGBA{R: 255, G: 255, B: 0, A: 255}
	case "2":
		return color.RGBA{R: 255, G: 200, B: 100, A: 255}
	case "1":
		return color.RGBA{R: 100, G: 150, B: 255, A: 255}
	default:
		return theme.Color(theme.ColorNameForeground)
	}
}

func buildConfig(myApp fyne.App) data.Config {
	prefs := myApp.Preferences()

	cfg := data.NewConfig()
	cfg.URL = prefs.String("ZABBIX_URL")
	cfg.Token = prefs.String("ZABBIX_TOKEN")
	cfg.User = prefs.String("ZABBIX_USER")
	cfg.Password = prefs.String("ZABBIX_PASS")
	cfg.SelfSigned = prefs.BoolWithFallback("SELF_SIGNED", false)

	if l, err := strconv.Atoi(prefs.StringWithFallback("PROBLEM_LIMIT", "200")); err == nil && l > 0 {
		cfg.Limit = l
	}

	return cfg
}

func main() {
	myApp := app.NewWithID("com.zabbix.android.monitor")

	currentTheme := myApp.Preferences().StringWithFallback("THEME", "dark")
	if currentTheme == "light" {
		myApp.Settings().SetTheme(theme.LightTheme())
	} else {
		myApp.Settings().SetTheme(theme.DarkTheme())
	}

	data.SetLang(myApp.Preferences().StringWithFallback("LANG", "ru"))

	window := myApp.NewWindow(data.Tr("app_title"))
	window.Resize(fyne.NewSize(450, 650))

	statusBind := binding.NewString()
	statusBind.Set(data.Tr("waiting_data"))

	statusLabel := widget.NewLabelWithData(statusBind)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	list := widget.NewList(
		func() int {
			return len(problemsData)
		},
		func() fyne.CanvasObject {
			line := canvas.NewRectangle(color.White)
			line.SetMinSize(fyne.NewSize(6, 0))

			timeLabel := widget.NewLabel("")
			timeLabel.TextStyle = fyne.TextStyle{Italic: true}

			hostLabel := widget.NewLabel("")
			hostLabel.TextStyle = fyne.TextStyle{Bold: true}
			hostLabel.Wrapping = fyne.TextWrapWord

			problemLabel := widget.NewLabel("")
			problemLabel.Wrapping = fyne.TextWrapWord

			cardContent := container.NewVBox(timeLabel, hostLabel, problemLabel)
			paddedContent := container.NewPadded(cardContent)

			return container.NewBorder(nil, nil, line, nil, paddedContent)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(problemsData) {
				return
			}
			p := problemsData[id]

			root := obj.(*fyne.Container)

			var line *canvas.Rectangle
			var vbox *fyne.Container

			for _, o := range root.Objects {
				if r, ok := o.(*canvas.Rectangle); ok {
					line = r
				} else if c, ok := o.(*fyne.Container); ok {
					if len(c.Objects) > 0 {
						if v, ok := c.Objects[0].(*fyne.Container); ok {
							vbox = v
						}
					}
				}
			}

			if line != nil {
				line.FillColor = getSeverityColor(p.Severity)
				line.Refresh()
			}

			if vbox != nil && len(vbox.Objects) >= 3 {
				vbox.Objects[0].(*widget.Label).SetText(data.FormatTime(p.Clock))
				vbox.Objects[1].(*widget.Label).SetText(p.HostName)
				vbox.Objects[2].(*widget.Label).SetText(p.Name)
			}
		},
	)

	welcomeText := widget.NewRichText(&widget.TextSegment{
		Text:  data.Tr("waiting_data"),
		Style: widget.RichTextStyle{Alignment: fyne.TextAlignCenter, TextStyle: fyne.TextStyle{Bold: true}},
	})
	centeredWelcome := container.NewCenter(welcomeText)

	mainStack := container.NewStack(centeredWelcome, list)

	refreshFunc := func() {
		cfg := buildConfig(myApp)

		if cfg.URL == "" || cfg.Token == "" {
			statusBind.Set(data.Tr("configure_server"))
			return
		}

		go func() {
			problems, err := data.DataRequestAPI(cfg)
			if err != nil {
				statusBind.Set(data.Tr("api_error"))
				return
			}

			fyne.Do(func() {
				problemsData = problems
				list.Refresh()

				statusBind.Set(fmt.Sprintf(data.Tr("problems_count"), len(problems)))

				if len(problems) > 0 {
					centeredWelcome.Hide()
					list.Show()
				} else {
					welcomeText.Segments = []widget.RichTextSegment{&widget.TextSegment{
						Text:  data.Tr("all_good"),
						Style: widget.RichTextStyle{Alignment: fyne.TextAlignCenter},
					}}
					welcomeText.Refresh()
					list.Hide()
					centeredWelcome.Show()
				}
			})
		}()
	}

	settingsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		settingsWindow := fyne.CurrentApp().NewWindow(data.Tr("settings"))
		settingsWindow.Resize(fyne.NewSize(440, 680))
		settingsWindow.CenterOnScreen()

		urlEntry := widget.NewEntry()
		urlEntry.Text = myApp.Preferences().String("ZABBIX_URL")

		userEntry := widget.NewEntry()
		userEntry.Text = myApp.Preferences().String("ZABBIX_USER")

		passEntry := widget.NewPasswordEntry()
		passEntry.Text = myApp.Preferences().String("ZABBIX_PASS")

		tokenEntry := widget.NewPasswordEntry()
		tokenEntry.Text = myApp.Preferences().String("ZABBIX_TOKEN")

		selfSignedCheck := widget.NewCheck(data.Tr("self_signed"), nil)
		selfSignedCheck.SetChecked(myApp.Preferences().BoolWithFallback("SELF_SIGNED", false))

		intervalEntry := widget.NewEntry()
		intervalEntry.Text = myApp.Preferences().StringWithFallback("REFRESH_INTERVAL", "60")

		limitEntry := widget.NewEntry()
		limitEntry.Text = myApp.Preferences().StringWithFallback("PROBLEM_LIMIT", "200")

		themeSelect := widget.NewSelect([]string{"Dark", "Light"}, nil)
		if myApp.Preferences().String("THEME") == "light" {
			themeSelect.SetSelected("Light")
		} else {
			themeSelect.SetSelected("Dark")
		}

		langOptions := []string{"Русский", "English"}
		langSelect := widget.NewSelect(langOptions, nil)
		if data.CurrentLang() == "en" {
			langSelect.SetSelected("English")
		} else {
			langSelect.SetSelected("Русский")
		}

		urlErrorLabel := widget.NewLabel("")
		urlErrorLabel.Hide()

		formContent := container.NewVBox(
			widget.NewLabel(data.Tr("url_server")),
			urlEntry,
			urlErrorLabel,
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
		)

		cancelBtn := widget.NewButton(data.Tr("cancel"), func() {
			settingsWindow.Close()
		})

		saveBtn := widget.NewButton(data.Tr("save"), func() {
			urlErrorLabel.Hide()

			if urlEntry.Text == "" {
				urlErrorLabel.SetText(data.Tr("error_url_required"))
				urlErrorLabel.Show()
				return
			}

			if tokenEntry.Text == "" {
				tokenEntry.SetError(data.Tr("error_token_required"))
				return
			}
			tokenEntry.SetError("")

			if intervalEntry.Text != "" {
				if _, err := strconv.Atoi(intervalEntry.Text); err != nil {
					intervalEntry.SetError(data.Tr("error_invalid_number"))
					return
				}
			}
			intervalEntry.SetError("")

			if limitEntry.Text != "" {
				if v, err := strconv.Atoi(limitEntry.Text); err != nil || v <= 0 {
					limitEntry.SetError(data.Tr("error_invalid_number"))
					return
				}
			}
			limitEntry.SetError("")

			myApp.Preferences().SetString("ZABBIX_URL", urlEntry.Text)
			myApp.Preferences().SetString("ZABBIX_USER", userEntry.Text)
			myApp.Preferences().SetString("ZABBIX_PASS", passEntry.Text)
			myApp.Preferences().SetString("ZABBIX_TOKEN", tokenEntry.Text)
			myApp.Preferences().SetBool("SELF_SIGNED", selfSignedCheck.Checked)
			myApp.Preferences().SetString("REFRESH_INTERVAL", intervalEntry.Text)
			myApp.Preferences().SetString("PROBLEM_LIMIT", limitEntry.Text)

			if langSelect.Selected == "English" {
				data.SetLang("en")
				myApp.Preferences().SetString("LANG", "en")
			} else {
				data.SetLang("ru")
				myApp.Preferences().SetString("LANG", "ru")
			}

			if themeSelect.Selected == "Light" {
				myApp.Settings().SetTheme(theme.LightTheme())
				myApp.Preferences().SetString("THEME", "light")
			} else {
				myApp.Settings().SetTheme(theme.DarkTheme())
				myApp.Preferences().SetString("THEME", "dark")
			}

			if refreshBtn != nil {
				refreshBtn.SetText(data.Tr("update"))
			}

			settingsWindow.Close()
			refreshFunc()
		})

		buttons := container.NewGridWithColumns(2, cancelBtn, saveBtn)

		content := container.NewBorder(
			formContent,
			buttons,
			nil, nil, nil,
		)

		settingsWindow.SetContent(container.NewPadded(content))
		settingsWindow.Show()
	})

	go func() {
		for {
			intervalStr := myApp.Preferences().StringWithFallback("REFRESH_INTERVAL", "60")
			interval, _ := strconv.Atoi(intervalStr)
			if interval <= 0 {
				interval = 60
			}
			time.Sleep(time.Duration(interval) * time.Second)
			refreshFunc()
		}
	}()

	refreshBtn = widget.NewButtonWithIcon(data.Tr("update"), theme.ViewRefreshIcon(), refreshFunc)

	topBar := container.NewHBox(statusLabel, layout.NewSpacer(), settingsBtn)
	content := container.NewBorder(
		container.NewVBox(topBar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), refreshBtn),
		nil, nil, mainStack,
	)

	window.SetContent(content)
	refreshFunc()
	window.ShowAndRun()
}
