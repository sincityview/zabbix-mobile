package data

import "sync"

var (
	currentLang = "ru"
	langMu      sync.RWMutex
)

var Translations = map[string]map[string]string{
	"ru": {
		"app_title":            "Zabbix Monitor",
		"settings":             "Настройки Zabbix Monitor",
		"url_server":           "URL сервера",
		"username":             "Пользователь",
		"password":             "Пароль",
		"token":                "Токен",
		"self_signed":          "Самоподписанный сертификат",
		"refresh_interval":     "Интервал обновления (сек)",
		"problem_limit":        "Лимит отображения проблем",
		"theme":                "Тема",
		"language":             "Язык",
		"cancel":               "Отмена",
		"save":                 "Сохранить",
		"update":               "Обновить",
		"problems_count":       "Проблем: %d",
		"all_good":             "Все системы в норме!",
		"waiting_data":         "Ожидание данных...",
		"configure_server":     "Настройте сервер",
		"api_error":            "Ошибка API",
		"error_url_required":   "URL обязателен",
		"error_token_required": "Токен обязателен",
		"error_invalid_number": "Некорректное число",
	},
	"en": {
		"app_title":            "Zabbix Monitor",
		"settings":             "Zabbix Monitor Settings",
		"url_server":           "Server URL",
		"username":             "Username",
		"password":             "Password",
		"token":                "Token",
		"self_signed":          "Self-signed certificate",
		"refresh_interval":     "Refresh interval (sec)",
		"problem_limit":        "Problems display limit",
		"theme":                "Theme",
		"language":             "Language",
		"cancel":               "Cancel",
		"save":                 "Save",
		"update":               "Refresh",
		"problems_count":       "Problems: %d",
		"all_good":             "All systems are operational!",
		"waiting_data":         "Waiting for data...",
		"configure_server":     "Configure server",
		"api_error":            "API Error",
		"error_url_required":   "URL is required",
		"error_token_required": "Token is required",
		"error_invalid_number": "Invalid number",
	},
}

func CurrentLang() string {
	langMu.RLock()
	defer langMu.RUnlock()
	return currentLang
}

func SetLang(lang string) {
	langMu.Lock()
	defer langMu.Unlock()
	currentLang = lang
}

func Tr(key string) string {
	langMu.RLock()
	lang := currentLang
	langMu.RUnlock()

	if texts, ok := Translations[lang]; ok {
		if text, ok := texts[key]; ok {
			return text
		}
	}

	if texts, ok := Translations["ru"]; ok {
		if text, ok := texts[key]; ok {
			return text
		}
	}
	return key
}
