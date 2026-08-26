package debug

import (
	"reflect"
	"strings"
	"testing"
)

// TestRunUnsupported проверяет ошибку для языка, отладка которого не реализована.
func TestRunUnsupported(t *testing.T) {
	err := Run(Options{Language: "python"})
	if err == nil {
		t.Error("ожидалась ошибка для неподдерживаемого языка")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("сообщение об ошибке должно содержать 'not supported', получили: %v", err)
	}
}

// TestGoDebugArgs проверяет формирование аргументов dlv debug в headless-режиме.
func TestGoDebugArgs(t *testing.T) {
	const dapAddr = "localhost:2345"
	base := []string{
		"debug",
		"--headless",
		"--listen=localhost:2345",
		"--api-version=2",
		"--accept-multiclient",
		"--allow-non-terminal-interactive=true",
	}

	cases := []struct {
		name   string
		target string
		params []string
		want   []string
	}{
		{
			name:   "без параметров",
			target: ".",
			want:   append(append([]string{}, base...), "."),
		},
		{
			name:   "с параметрами",
			target: "./cmd/app",
			params: []string{"serve", "--addr=:8080"},
			want:   append(append([]string{}, base...), "./cmd/app", "--", "serve", "--addr=:8080"),
		},
		{
			name:   "пустая цель и параметры",
			target: "main",
			params: nil,
			want:   append(append([]string{}, base...), "main"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := goDebugArgs(dapAddr, tc.target, tc.params)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("goDebugArgs(%q, %q, %v) = %v, want %v", dapAddr, tc.target, tc.params, got, tc.want)
			}
		})
	}
}

// TestDebugDir проверяет преобразование каталога main-файла в путь для Delve:
// корень остаётся точкой, подпапки получают префикс "./".
func TestDebugDir(t *testing.T) {
	cases := []struct {
		target string
		want   string
	}{
		{"main.go", "."},
		{"cmd/dev/main.go", "./cmd/dev"},
		{"cmd/app/main.go", "./cmd/app"},
		{"./cmd/dev/main.go", "./cmd/dev"},
	}
	for _, tc := range cases {
		if got := debugDir(tc.target); got != tc.want {
			t.Errorf("debugDir(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}
