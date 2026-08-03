package data

import (
	"sync"
	"testing"
)

func TestTr_Russian(t *testing.T) {
	SetLang("ru")
	result := Tr("app_title")
	if result != "Zabbix Monitor" {
		t.Errorf("Tr('app_title') = %q, want 'Zabbix Monitor'", result)
	}
}

func TestTr_English(t *testing.T) {
	SetLang("en")
	result := Tr("save")
	if result != "Save" {
		t.Errorf("Tr('save') = %q, want 'Save'", result)
	}
	SetLang("ru")
}

func TestTr_FallbackToRussian(t *testing.T) {
	SetLang("de")
	result := Tr("cancel")
	if result != "Отмена" {
		t.Errorf("Tr('cancel') with unknown lang = %q, want 'Отмена'", result)
	}
	SetLang("ru")
}

func TestTr_UnknownKey(t *testing.T) {
	SetLang("ru")
	result := Tr("nonexistent_key")
	if result != "nonexistent_key" {
		t.Errorf("Tr('nonexistent_key') = %q, want 'nonexistent_key'", result)
	}
}

func TestCurrentLang(t *testing.T) {
	SetLang("en")
	if CurrentLang() != "en" {
		t.Errorf("CurrentLang() = %q, want 'en'", CurrentLang())
	}
	SetLang("ru")
}

func TestConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				SetLang("en")
			} else {
				SetLang("ru")
			}
			_ = Tr("app_title")
			_ = CurrentLang()
		}(i)
	}
	wg.Wait()
}
