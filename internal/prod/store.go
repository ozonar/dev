package prod

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// reportsDir — каталог для хранения отчётов.
const reportsDir = "/etc/prod-command/reports"

// ReportPath возвращает путь для сохранения отчёта по времени.
func ReportPath(t time.Time) string {
	return filepath.Join(reportsDir, t.Format("2006-01-02_15-04-05")+".json")
}

// SaveReport сохраняет отчёт в reportsDir и возвращает путь.
func SaveReport(rep *Report) (string, error) {
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return "", fmt.Errorf("create reports dir: %w", err)
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	path := ReportPath(rep.Timestamp)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// LoadPrevious находит последний отчёт до указанного времени.
func LoadPrevious(before time.Time) (*Report, error) {
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	for i := len(files) - 1; i >= 0; i-- {
		name := files[i]
		t, perr := time.Parse("2006-01-02_15-04-05", trimExt(name))
		if perr != nil {
			continue
		}
		if t.Before(before) {
			return loadReport(filepath.Join(reportsDir, name))
		}
	}
	return nil, fmt.Errorf("no previous report found")
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

func loadReport(path string) (*Report, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}
