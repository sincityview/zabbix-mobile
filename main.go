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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"zabbix/data"
)

var (
	problemsData []data.Problem
)

func getSeverityColor(severity string) color.Color {
	switch severity {
	case "5": // Disaster
		return color.RGBA{R: 220, G: 0, B: 0, A: 255}
	case "4": // High
		return color.RGBA{R: 255, G: 153, B: 0, A: 255}
	case "3": // Average
		return color.RGBA{R: 255, G: 255, B: 0, A: 255}
	case "2": // Warning
		return color.RGBA{R: 255, G: 200, B: 100, A: 255}
	case "1": // Information
		return color.RGBA{R: 100, G: 150, B: 255, A: 255}
	default:
		return theme.ForegroundColor()
	}
}

func main() {
	myApp := app.NewWithID("com.zabbix.mobile.monitor")

	currentTheme := myApp.Preferences().StringWithFallback("THEME", "dark")
	if currentTheme == "light" {
		myApp.Settings().SetTheme(theme.LightTheme())
	} else {
		myApp.Settings().SetTheme(theme.DarkTheme())
	}

	window := myApp.NewWindow("Zabbix Monitor")
	window.Resize(fyne.NewSize(450, 650))

	statusBind := binding.NewString()
	statusBind.Set("Обновление...")

	statusLabel := widget.NewLabelWithData(statusBind)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	list := widget.NewList(
		func() int {
			return len(problemsData)
		},
		func() fyne.CanvasObject {
			line := canvas.NewRectangle(color.White)
			line.SetMinSize(fyne.NewSize(6, 0))

			// timeLabel := widget.NewLabel("")
			timeLabel.TextStyle = fyne.TextStyle{Italic: true}

			// hostLabel := widget.NewLabel("")
			hostLabel.TextStyle = fyne.TextStyle{Bold: true}
			hostLabel.Wrapping = fyne.TextWrapWord

			// problemLabel := widget.NewLabel("")
			problemLabel.Wrapping = fyne.TextWrapWord

			cardContent := container.NewVBox(timeLabel, hostLabel, problemLabel)
			return container.NewBorder(nil, nil, line, nil, container.NewPadded(cardContent))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			p := problemsData[id]
			root := obj.(*fyne.Container)

			line := root.Objects[0].(*canvas.Rectangle)
			line.FillColor = getSeverityColor(p.Severity)
			line.Refresh()

			padded := root.Objects[1].(*fyne.Container)
			vbox := padded.Objects[0].(*fyne.Container)

			vbox.Objects[0].(*widget.Label).SetText(data.FormatTime(p.Clock))
			vbox.Objects[1].(*widget.Label).SetText(p.HostName)
			vbox.Objects[2].(*widget.Label).SetText(p.Name)
		},
	)

	welcomeText := widget.NewRichText(&widget.TextSegment{
		Text:  "Ожидание данных...",
		Style: widget.RichTextStyle{Alignment: fyne.TextAlignCenter, TextStyle: fyne.TextStyle{Bold: true}},
	})
	centeredWelcome := container.NewCenter(welcomeText)

	mainStack := container.NewStack(centeredWelcome, list)

	refreshFunc := func() {
		u := myApp.Preferences().String("ZABBIX_URL")
		t := myApp.Preferences().String("ZABBIX_TOKEN")

		if u == "" || t == "" {
			statusBind.Set("Настройте сервер")
			return
		}

		go func() {
			problems, err := data.DataRequestAPI(u, t)
			if err != nil {
				statusBind.Set("Ошибка API")
				return
			}

			fyne.Do(func() {
				problemsData = problems
				list.Refresh()

				statusBind.Set(fmt.Sprintf("Проблем: %d", len(problems)))

				if len(problems) > 0 {
					centeredWelcome.Hide()
					list.Show()
				} else {
					welcomeText.Segments = []widget.RichTextSegment{&widget.TextSegment{
						Text:  "Все системы в норме!",
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
		urlEntry := widget.NewEntry()
		urlEntry.Text = myApp.Preferences().String("ZABBIX_URL")
		tokenEntry := widget.NewPasswordEntry()
		tokenEntry.Text = myApp.Preferences().String("ZABBIX_TOKEN")
		intervalEntry := widget.NewEntry()
		intervalEntry.Text = myApp.Preferences().StringWithFallback("REFRESH_INTERVAL", "60")

		themeSelect := widget.NewSelect([]string{"Dark", "Light"}, nil)
		themeSelect.SetSelected("Dark")
		if myApp.Preferences().String("THEME") == "light" {
			themeSelect.SetSelected("Light")
		}

		form := widget.NewForm(
			widget.NewFormItem("URL сервера", urlEntry),
			widget.NewFormItem("Токен", tokenEntry),
			widget.NewFormItem("Интервал (сек)", intervalEntry),
			widget.NewFormItem("Тема", themeSelect),
		)

		d := dialog.NewCustomConfirm("Настройки", "Сохранить", "Отмена", form, func(confirm bool) {
			if confirm {
				myApp.Preferences().SetString("ZABBIX_URL", urlEntry.Text)
				myApp.Preferences().SetString("ZABBIX_TOKEN", tokenEntry.Text)
				myApp.Preferences().SetString("REFRESH_INTERVAL", intervalEntry.Text)

				if themeSelect.Selected == "Light" {
					myApp.Settings().SetTheme(theme.LightTheme())
					myApp.Preferences().SetString("THEME", "light")
				} else {
					myApp.Settings().SetTheme(theme.DarkTheme())
					myApp.Preferences().SetString("THEME", "dark")
				}
				refreshFunc()
			}
		}, window)
		d.Resize(fyne.NewSize(400, 350))
		d.Show()
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

	refreshBtn := widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), refreshFunc)

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
