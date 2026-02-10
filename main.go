package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"zabbix/internal/zabbix"
)

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

	os.Setenv("ZABBIX_URL", myApp.Preferences().String("ZABBIX_URL"))
	os.Setenv("ZABBIX_TOKEN", myApp.Preferences().String("ZABBIX_TOKEN"))

	statusLabel := widget.NewLabel("Проблем: 0")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	richText := widget.NewRichText()
	richText.Wrapping = fyne.TextWrapWord
	scroll := container.NewVScroll(richText)
	scroll.Hide()

	welcomeText := widget.NewRichText(&widget.TextSegment{
		Text: "📡 Ожидание данных\nНажмите кнопку 'Обновить'",
		Style: widget.RichTextStyle{Alignment: fyne.TextAlignCenter, TextStyle: fyne.TextStyle{Bold: true}},
	})
	centeredWelcome := container.NewCenter(welcomeText)
	mainStack := container.NewStack(centeredWelcome, scroll)

	refreshFunc := func() {
		centeredWelcome.Hide()
		scroll.Show()
		go func() {
			problems, err := zabbix.DataRequestAPI()
			if err != nil {
				statusLabel.SetText("Ошибка API")
			} else {
				statusLabel.SetText(fmt.Sprintf("Проблем: %d", len(problems)))
				var segments []widget.RichTextSegment
				if len(problems) == 0 {
					segments = append(segments, &widget.TextSegment{Text: "✅ Все системы в норме!", Style: widget.RichTextStyle{ColorName: theme.ColorNameSuccess}})
				} else {
					for i, p := range problems {
						if i > 0 { segments = append(segments, &widget.SeparatorSegment{}) }
						segments = append(segments, &widget.TextSegment{Text: "\n" + zabbix.FormatTime(p.Clock) + "\n", Style: widget.RichTextStyleStrong})
						segments = append(segments, &widget.TextSegment{Text: p.HostName + "\n", Style: widget.RichTextStyleStrong})
						segments = append(segments, &widget.TextSegment{Text: p.Name + "\n", Style: widget.RichTextStyle{ColorName: theme.ColorNameWarning}})
					}
				}
				richText.Segments = segments
				richText.Refresh()
			}
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
		if myApp.Preferences().StringWithFallback("THEME", "dark") == "light" {
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
				
				os.Setenv("ZABBIX_URL", urlEntry.Text)
				os.Setenv("ZABBIX_TOKEN", tokenEntry.Text)

				if themeSelect.Selected == "Light" {
					myApp.Settings().SetTheme(theme.LightTheme())
					myApp.Preferences().SetString("THEME", "light")
				} else {
					myApp.Settings().SetTheme(theme.DarkTheme())
					myApp.Preferences().SetString("THEME", "dark")
				}
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
