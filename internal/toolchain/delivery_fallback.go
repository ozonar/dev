package toolchain

import "fmt"

// fallbackDelivery — правила доставки для неподдерживаемых комбинаций
// (windows и пр.). Линтеры качаются linux-версиями (как было до введения
// адаптеров), PHP не скачивается — даётся понятная ошибка.
type fallbackDelivery struct{}

func (fallbackDelivery) PhpBinaryName(version string) string { return "usr/bin/php" + version }
func (fallbackDelivery) PhpSource() phpSource                { return fallbackPhpSource{} }
func (fallbackDelivery) BiomeAsset() string                  { return "biome-linux-x64" }
func (fallbackDelivery) RuffTarget() string                  { return "x86_64-unknown-linux-gnu" }

// fallbackPhpSource — заглушка источника PHP: на таких ОС php не скачивается.
type fallbackPhpSource struct{}

func (fallbackPhpSource) resolve(required string) (resolved, url, archive string, err error) {
	return "", "", "", fmt.Errorf("PHP download is not supported on this OS; install PHP manually")
}
