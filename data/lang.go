package data

import (
	"strings"
	"sync/atomic"
)

// currentLang хранит текущий язык, а messages — уже собранный словарь
// «ключ → текст» (выбранный язык поверх русского как резервного).
//
// Так Tr() — это одно чтение atomic.Value и один поиск по map, без RWMutex и
// без двойного поиска на каждый вызов (а вызывается он на каждой отрисовке).
var (
	currentLang atomic.Value // string
	messages    atomic.Value // map[string]string
)

func init() {
	currentLang.Store("ru")
	messages.Store(buildMessages("ru"))
}

var Translations = map[string]map[string]string{
	"ru": {
		"app_title":                  "Zabbix Monitor",
		"settings":                   "Настройки Zabbix Monitor",
		"url_server":                 "URL сервера",
		"username":                   "Пользователь",
		"password":                   "Пароль",
		"token":                      "Токен",
		"self_signed":                "Самоподписанный сертификат",
		"refresh_interval":           "Интервал обновления (сек)",
		"problem_limit":              "Лимит отображения проблем",
		"theme":                      "Тема",
		"language":                   "Язык",
		"cancel":                     "Отмена",
		"save":                       "Сохранить",
		"update":                     "Обновить",
		"problems_count":             "Проблем: %d",
		"all_good":                   "Все системы в норме!",
		"waiting_data":               "Загрузка…",
		"configure_server":           "Настройте сервер",
		"api_error":                  "Ошибка API",
		"error_url_required":         "URL обязателен",
		"error_token_required":       "Токен обязателен",
		"error_invalid_number":       "Некорректное число",
		"error_url_invalid":          "Некорректный URL (пример: https://server/api_jsonrpc.php)",
		"error_credentials_required": "Нужен токен или логин с паролем",
		"error_save_settings":        "Не удалось сохранить настройки: %s",
		"test_connection":            "Проверить подключение",
		"connection_ok":              "Подключение успешно, проблем: %d",
		"connection_failed":          "Ошибка подключения: %s",
		"no_host_names":              "нет имён хостов",
		"settings_file":              "Настройки: %s",
		"settings_restored":          "(восстановлено из резервной копии)",
		"reset_settings":             "Сбросить настройки",
		"reset_confirm":              "Удалить сохранённые адрес, токен и пароль с этого устройства?",
		"reset_done":                 "Настройки удалены",
		"cleartext_warning":          "Внимание: http:// передаёт токен по сети открытым текстом. Используйте https://",
		"cached_data":                "Кеш %s",
		"problem_details":            "Проблема",
		"close":                      "Закрыть",
		"time":                       "Время",
		"host":                       "Хост",
		"problem":                    "Проблема",
		"severity":                   "Важность",
		"acknowledged":               "Подтверждено",
		"yes":                        "да",
		"no":                         "нет",
		"severity_0":                 "Не классифицирована",
		"severity_1":                 "Информация",
		"severity_2":                 "Предупреждение",
		"severity_3":                 "Средняя",
		"severity_4":                 "Высокая",
		"severity_5":                 "Авария",
		"severity_unknown":           "Неизвестно",
	},
	"en": {
		"app_title":                  "Zabbix Monitor",
		"settings":                   "Zabbix Monitor Settings",
		"url_server":                 "Server URL",
		"username":                   "Username",
		"password":                   "Password",
		"token":                      "Token",
		"self_signed":                "Self-signed certificate",
		"refresh_interval":           "Refresh interval (sec)",
		"problem_limit":              "Problems display limit",
		"theme":                      "Theme",
		"language":                   "Language",
		"cancel":                     "Cancel",
		"save":                       "Save",
		"update":                     "Refresh",
		"problems_count":             "Problems: %d",
		"all_good":                   "All systems are operational!",
		"waiting_data":               "Loading…",
		"configure_server":           "Configure server",
		"api_error":                  "API Error",
		"error_url_required":         "URL is required",
		"error_token_required":       "Token is required",
		"error_invalid_number":       "Invalid number",
		"error_url_invalid":          "Invalid URL (example: https://server/api_jsonrpc.php)",
		"error_credentials_required": "Token or username with password is required",
		"error_save_settings":        "Failed to save settings: %s",
		"test_connection":            "Test connection",
		"connection_ok":              "Connection OK, problems: %d",
		"connection_failed":          "Connection failed: %s",
		"no_host_names":              "no host names",
		"settings_file":              "Settings: %s",
		"settings_restored":          "(restored from backup)",
		"reset_settings":             "Reset settings",
		"reset_confirm":              "Delete the saved address, token and password from this device?",
		"reset_done":                 "Settings deleted",
		"cleartext_warning":          "Warning: http:// sends the token over the network in cleartext. Use https://",
		"cached_data":                "Cache %s",
		"problem_details":            "Problem",
		"close":                      "Close",
		"time":                       "Time",
		"host":                       "Host",
		"problem":                    "Problem",
		"severity":                   "Severity",
		"acknowledged":               "Acknowledged",
		"yes":                        "yes",
		"no":                         "no",
		"severity_0":                 "Not classified",
		"severity_1":                 "Information",
		"severity_2":                 "Warning",
		"severity_3":                 "Average",
		"severity_4":                 "High",
		"severity_5":                 "Disaster",
		"severity_unknown":           "Unknown",
	},
}

// buildMessages собирает словарь: русский как база, выбранный язык сверху.
func buildMessages(lang string) map[string]string {
	merged := make(map[string]string, len(Translations["ru"]))
	for key, text := range Translations["ru"] {
		merged[key] = text
	}
	if lang != "ru" {
		for key, text := range Translations[lang] {
			merged[key] = text
		}
	}
	return merged
}

// KnownLangs возвращает поддерживаемые языки.
func KnownLangs() []string { return []string{"ru", "en"} }

// CurrentLang возвращает текущий язык интерфейса.
func CurrentLang() string {
	if v, ok := currentLang.Load().(string); ok {
		return v
	}
	return "ru"
}

// SetLang переключает язык интерфейса (неизвестный язык трактуется как ru).
func SetLang(lang string) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if _, ok := Translations[lang]; !ok {
		lang = "ru"
	}
	currentLang.Store(lang)
	messages.Store(buildMessages(lang))
}

// Tr возвращает перевод по ключу (или сам ключ, если перевода нет).
func Tr(key string) string {
	if dict, ok := messages.Load().(map[string]string); ok {
		if text, ok := dict[key]; ok {
			return text
		}
	}
	return key
}
