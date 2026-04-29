package data

var CurrentLang = "ru"

var Translations = map[string]map[string]string{
	"ru": {
		"app_title":          "Zabbix Monitor",
		"settings":           "Настройки Zabbix Monitor",
		"url_server":         "URL сервера",
		"token":              "Token",
		"self_signed":        "Self-signed сертификат (InsecureSkipVerify)",
		"refresh_interval":   "Интервал обновления (сек)",
		"problem_limit":      "Лимит отображения проблем",
		"theme":              "Тема",
		"language":           "Язык",
		"cancel":             "Отмена",
		"save":               "Сохранить",
		"update":             "Обновить",
		"problems_count":     "Проблем: %d",
		"all_good":           "Все системы в норме!",
		"waiting_data":       "Ожидание данных...",
		"configure_server":   "Настройте сервер",
		"api_error":          "Ошибка API",
	},
	"en": {
		"app_title":          "Zabbix Monitor",
		"settings":           "Zabbix Monitor Settings",
		"url_server":         "Server URL",
		"token":              "Token",
		"self_signed":        "Self-signed certificate (InsecureSkipVerify)",
		"refresh_interval":   "Refresh interval (sec)",
		"problem_limit":      "Problems display limit",
		"theme":              "Theme",
		"language":           "Language",
		"cancel":             "Cancel",
		"save":               "Save",
		"update":             "Refresh",
		"problems_count":     "Problems: %d",
		"all_good":           "All systems are operational!",
		"waiting_data":       "Waiting for data...",
		"configure_server":   "Configure server",
		"api_error":          "API Error",
	},
}

func Tr(key string) string {
	if lang, ok := Translations[CurrentLang]; ok {
		if text, ok := lang[key]; ok {
			return text
		}
	}

	if text, ok := Translations["ru"][key]; ok {
		return text
	}
	return key
}