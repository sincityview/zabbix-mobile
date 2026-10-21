package data

import (
	"errors"
	"os"
	"path/filepath"
)

// errFileTooBig защищает от чтения мусора вместо небольшого JSON-файла.
var errFileTooBig = errors.New("file: слишком большой файл")

// maxStateFileSize — предельный размер файлов состояния (настройки, снапшот).
const maxStateFileSize = 4 << 20

// writeFileSynced пишет файл целиком и сбрасывает его на диск.
func writeFileSynced(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// syncDir делает fsync каталога: без него переименование может не пережить
// сбой питания, даже если сам файл уже сброшен на диск.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// writeAtomic записывает data в path через временный файл: tmp → fsync →
// rename → fsync каталога.
//
// Такой сбой, как убийство процесса или разрядка телефона посреди записи, не
// оставляет пустой или обрезанный файл — на диске всегда либо старая, либо
// новая версия целиком.
func writeAtomic(path string, data []byte, perm os.FileMode, dirSync bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := writeFileSynced(tmp, data, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if dirSync {
		syncDir(dir)
	}
	return nil
}

// readStateFile читает файл состояния с ограничением размера.
func readStateFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxStateFileSize {
		return nil, errFileTooBig
	}
	return os.ReadFile(path)
}
