package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"image/color"
	"fyne.io/fyne/v2/canvas"

	"zabbix/data" 
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
	statusBind.Set("Проблем: 0")

	statusLabel := widget.NewLabelWithData(statusBind)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	problemsContainer := container.NewVBox()
	scroll := container.NewVScroll(problemsContainer)
	scroll.SetMinSize(fyne.NewSize(400, 500)) 
	scroll.Hide()

	welcomeText := widget.NewRichText(&widget.TextSegment{
		Text: "📡 Ожидание данных\nНажмите кнопку 'Обновить'",
		Style: widget.RichTextStyle{Alignment: fyne.TextAlignCenter, TextStyle: fyne.TextStyle{Bold: true}},
	})
	centeredWelcome := container.NewCenter(welcomeText)
	mainStack := container.NewStack(centeredWelcome, scroll)

	refreshFunc := func() {
		u := myApp.Preferences().String("ZABBIX_URL")
		t := myApp.Preferences().String("ZABBIX_TOKEN")

		if u == "" || t == "" {
			statusBind.Set("Настройте URL и Токен")
			return
		}

		go func() {
			problems, err := data.DataRequestAPI(u, t)
			if err != nil {
				statusBind.Set("Ошибка API")
				return
			}

			fyne.Do(func() {
				centeredWelcome.Hide()
				scroll.Show()

				problemsContainer.Objects = nil 
				statusBind.Set(fmt.Sprintf("Проблем: %d", len(problems)))

				if len(problems) == 0 {
					problemsContainer.Add(widget.NewLabelWithStyle("✅ Все системы в норме!", 
						fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
				} else {
					for _, p := range problems {
						line := canvas.NewRectangle(getSeverityColor(p.Severity))
						line.SetMinSize(fyne.NewSize(6, 0)) 

						hostLabel := widget.NewLabelWithStyle(p.HostName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
						hostLabel.Wrapping = fyne.TextWrapWord

						problemLabel := widget.NewLabel(p.Name)
						problemLabel.Wrapping = fyne.TextWrapWord
						
						timeLabel := widget.NewLabelWithStyle(data.FormatTime(p.Clock), fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

						cardContent := container.NewVBox(timeLabel, hostLabel, problemLabel)
						
						paddedContent := container.NewPadded(cardContent)

						card := container.NewBorder(nil, nil, line, nil, paddedContent)

						problemsContainer.Add(card)
						problemsContainer.Add(widget.NewSeparator())
					}
				}
				
				problemsContainer.Refresh()
				scroll.Refresh()
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
		if myApp.Preferences().String("THEME") == "light" {
			themeSelect.SetSelected("Light")
		} else {
			themeSelect.SetSelected("Dark")
		}

		form := widget.NewForm(
			widget.NewFormItem("URL сервера", urlEntry),
			widget.NewFormItem("Токен", tokenEntry),
			widget.NewFormItem("Автообновление (сек)", intervalEntry),
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
			interval, err := strconv.Atoi(intervalStr)
			if err != nil || interval <= 0 {
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
	window.ShowAndRun()
}
