package data

import (
	"encoding/json"
	"strings"
)

// Secret — строка с секретом (токен, пароль).
//
// Печатается как маска ("***"), поэтому случайный лог или сообщение об ошибке
// через fmt не сливает секрет. JSON-сериализация при этом остаётся полной,
// иначе настройки не сохранить.
type Secret string

// Value возвращает реальное значение секрета.
func (s Secret) Value() string { return string(s) }

// IsSet сообщает, что секрет задан (не пустой и не из пробелов).
func (s Secret) IsSet() bool { return strings.TrimSpace(string(s)) != "" }

// String маскирует секрет — используется fmt для %v/%s.
func (s Secret) String() string {
	if s.IsSet() {
		return "***"
	}
	return ""
}

// GoString маскирует секрет для %#v.
func (s Secret) GoString() string { return s.String() }

// MarshalJSON пишет настоящее значение: файл настроек должен быть пригодным.
func (s Secret) MarshalJSON() ([]byte, error) {
	if s == "" {
		return []byte(`""`), nil
	}
	return json.Marshal(string(s))
}

// UnmarshalJSON читает значение из файла настроек.
func (s *Secret) UnmarshalJSON(raw []byte) error {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*s = Secret(v)
	return nil
}
