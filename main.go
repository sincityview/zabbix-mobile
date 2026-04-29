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
		return theme.Color(theme.ColorNameForeground)
	}
}

func main() {
	myApp := app.NewWithID("com.zabbix.android.monitor")

	currentTheme := myApp.Preferences().StringWithFallback("THEME", "dark")
	if currentTheme == "light" {
		myApp.Settings().SetTheme(theme.LightTheme())
	} else {
		myApp.Settings().SetTheme(theme.DarkTheme())
	}

	window := myApp.NewWindow(data.Tr("app_title"))
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
		u := myApp.Preferences().String("ZABBIX_URL")
		t := myApp.Preferences().String("ZABBIX_TOKEN")

		if u == "" || t == "" {
			statusBind.Set(data.Tr("configure_server"))
			return
		}

		go func() {
			problems, err := data.DataRequestAPI(u, t)
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
		urlEntry.SetPlaceHolder("")

		tokenEntry := widget.NewPasswordEntry()
		tokenEntry.Text = myApp.Preferences().String("ZABBIX_TOKEN")
		urlEntry.SetPlaceHolder("")

		selfSignedCheck := widget.NewCheck(data.Tr("self_signed"), nil)
		selfSignedCheck.SetChecked(myApp.Preferences().BoolWithFallback("SELF_SIGNED", false))

		intervalEntry := widget.NewEntry()
		intervalEntry.Text = myApp.Preferences().StringWithFallback("REFRESH_INTERVAL", "60")

		limitEntry := widget.NewEntry()
		limitEntry.Text = myApp.Preferences().StringWithFallback("PROBLEM_LIMIT", "200")

		themeSelect := widget.NewSelect([]string{"Dark", "Light"}, nil)
		themeSelect.SetSelected("Dark")
		if myApp.Preferences().String("THEME") == "light" {
			themeSelect.SetSelected("Light")
		}

		langOptions := []string{"Русский", "English"}
		langSelect := widget.NewSelect(langOptions, nil)
		if data.CurrentLang == "en" {
			langSelect.SetSelected("English")
		} else {
			langSelect.SetSelected("Русский")
		}

		formContent := container.NewVBox(
			widget.NewLabel(data.Tr("url_server")),
			urlEntry,
			widget.NewLabel(data.Tr("token")),
			tokenEntry,
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
			myApp.Preferences().SetString("ZABBIX_URL", urlEntry.Text)
			myApp.Preferences().SetString("ZABBIX_TOKEN", tokenEntry.Text)
			myApp.Preferences().SetBool("SELF_SIGNED", selfSignedCheck.Checked)
			myApp.Preferences().SetString("REFRESH_INTERVAL", intervalEntry.Text)
			myApp.Preferences().SetString("PROBLEM_LIMIT", limitEntry.Text)

			if langSelect.Selected == "English" {
				data.CurrentLang = "en"
			} else {
				data.CurrentLang = "ru"
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
