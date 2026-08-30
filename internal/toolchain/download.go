package toolchain

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// download скачивает программу в папку инструментов. Установка выполняется
// атомарно: архив (или простой файл) сначала раскладывается во временную папку
// внутри папки инструментов, и только после полного успеха переносится в
// целевую папку версии. Это исключает появление «установленной», но битой
// версии при прерывании загрузки или распаковки.
func (m *Manager) download(ex Executable) error {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", m.dir, err)
	}

	tmpFile, err := os.CreateTemp(m.dir, "download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpName)
	}()

	resp, err := http.Get(ex.URL())
	if err != nil {
		return fmt.Errorf("failed to download %s: %v", ex.Name(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: HTTP %d", ex.Name(), resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("error writing %s: %v", ex.Name(), err)
	}
	tmpFile.Close()

	// Распаковываем во временную папку в корне папки инструментов (та же ФС,
	// что и целевая, поэтому финальный rename атомарен). Папка .install-* не
	// попадает в downloadedVersions, которая смотрит только подпапки <имя>/.
	tmpDir, err := os.MkdirTemp(m.dir, ".install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	switch ex.Archive() {
	case "tar.gz":
		if err := extractTarGzAll(tmpName, tmpDir); err != nil {
			return fmt.Errorf("error extracting %s: %v", ex.Name(), err)
		}
	case "tar.xz":
		if err := extractTarXZAll(tmpName, tmpDir); err != nil {
			return fmt.Errorf("error extracting %s: %v", ex.Name(), err)
		}
	case "zip":
		if err := extractZipAll(tmpName, tmpDir); err != nil {
			return fmt.Errorf("error extracting %s: %v", ex.Name(), err)
		}
	default:
		// Простой файл — кладём в корень временной папки.
		target := filepath.Join(tmpDir, filepath.Base(ex.Binary()))
		if err := copyFile(tmpName, target); err != nil {
			return fmt.Errorf("error saving %s: %v", ex.Name(), err)
		}
	}

	// Даём права на выполнение исполняемому файлу.
	if err := os.Chmod(filepath.Join(tmpDir, ex.Binary()), 0755); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %v", ex.Name(), err)
	}

	// Атомарно переносим готовую папку в целевую папку версии.
	progDir := m.programDir(ex)
	if err := replaceDir(progDir, tmpDir); err != nil {
		return fmt.Errorf("failed to install %s: %v", ex.Name(), err)
	}

	return nil
}

// extractTarGzAll распаковывает tar.gz архив в директорию dest,
// сохраняя структуру. Защищает от выхода за пределы dest (path traversal).
func extractTarGzAll(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Формируем безопасный путь внутри dest.
		target := safeJoin(dest, hdr.Name)
		if target == "" {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := writeFromReader(tr, target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractTarXZAll распаковывает tar.xz архив в директорию dest,
// сохраняя структуру. Защищает от выхода за пределы dest (path traversal).
func extractTarXZAll(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	xr, err := xz.NewReader(f)
	if err != nil {
		return err
	}

	tr := tar.NewReader(xr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := safeJoin(dest, hdr.Name)
		if target == "" {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := writeFromReader(tr, target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractZipAll распаковывает zip архив в директорию dest,
// сохраняя структуру. Защищает от выхода за пределы dest (path traversal).
func extractZipAll(src, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		target := safeJoin(dest, f.Name)
		if target == "" {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode()
		err = writeFromReader(rc, target, os.FileMode(mode))
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// safeJoin объединяет dest и относительный путь name, гарантируя,
// что результат не выходит за пределы dest. Пустой результат означает
// небезопасный путь, который следует пропустить.
func safeJoin(dest, name string) string {
	if name == "" {
		return ""
	}
	// Нормализуем и отбрасываем абсолютные пути и выходы за пределы.
	clean := filepath.Clean(filepath.Join(dest, name))
	if clean == dest {
		return ""
	}
	if !strings.HasPrefix(clean, dest+string(filepath.Separator)) {
		return ""
	}
	return clean
}

// writeFromReader пишет данные из reader в файл target с правами mode.
func writeFromReader(r io.Reader, target string, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

// copyFile копирует файл src в dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
