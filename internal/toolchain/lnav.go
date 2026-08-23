package toolchain

import (
	"fmt"
	"runtime"
)

// lnavVersion — актуальная версия lnav. Обновляется вручную при выходе
// новых релизов.
const lnavVersion = "0.14.0"

// LnavProgram возвращает описание lnav — просмотрщика логов.
func LnavProgram() Executable {
	osPart := "linux-musl"
	switch runtime.GOOS {
	case "darwin":
		osPart = "macos"
	}

	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		// В именах архивов lnav архитектура arm64 называется по-разному:
		// aarch64 для macOS, arm64 для Linux.
		if runtime.GOOS == "darwin" {
			arch = "aarch64"
		} else {
			arch = "arm64"
		}
	}

	filename := fmt.Sprintf("lnav-%s-%s-%s.zip", lnavVersion, osPart, arch)
	url := fmt.Sprintf("https://github.com/tstack/lnav/releases/download/v%s/%s", lnavVersion, filename)

	return NewProgram("lnav", lnavVersion, "lnav", url, "zip", "{lnav}")
}
