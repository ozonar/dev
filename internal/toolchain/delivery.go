// Платформенные правила доставки бинарных артефактов.
//
// Каждая поддерживаемая комбинация ОС/архитектуры описывается отдельным
// типом-адаптером (linuxAmd64, darwinArm64 и т.д.), методы которого
// возвращают готовые имена артефактов без ветвлений. Выбор адаптера для
// текущей платформы выполняется декларативным реестром (мапой), куда
// файлы delivery_*.go регистрируют свои записи.
package toolchain

import "runtime"

// platformKey — пара ОС/архитектура, однозначно задающая правила доставки.
type platformKey struct{ goos, goarch string }

// Delivery — правила доставки бинарных артефактов для конкретной платформы.
// Методы возвращают готовые значения, поэтому реализации не содержат
// ветвлений по ОС или архитектуре.
type Delivery interface {
	// PhpBinaryName возвращает относительный путь к бинарю php внутри папки версии.
	PhpBinaryName(version string) string
	// PhpSource возвращает источник скачивания PHP для этой платформы.
	PhpSource() phpSource
	// BiomeAsset возвращает имя исполняемого файла Biome.
	BiomeAsset() string
	// RuffTarget возвращает triple целевой платформы Ruff.
	RuffTarget() string
}

// phpSource — источник скачивания PHP: определяет полную версию, URL и тип
// архива для требуемой major.minor версии. Реализации живут рядом со своими
// адаптерами (php-builder на Linux, static-php на macOS).
type phpSource interface {
	resolve(required string) (resolved, url, archive string, err error)
}

// deliveries — реестр правил доставки по комбинациям ОС/архитектуры.
// Заполняется в init-функциях файлов delivery_*.go.
var deliveries = make(map[platformKey]Delivery)

// registerDelivery регистрирует правила доставки для пары ОС/архитектуры.
func registerDelivery(goos, goarch string, d Delivery) {
	deliveries[platformKey{goos, goarch}] = d
}

// CurrentDelivery возвращает правила доставки для текущей платформы.
// Для неподдерживаемых комбинаций используется fallbackDelivery,
// сохраняющая прежнее поведение (linux-имена линтеров, php не скачивается).
func CurrentDelivery() Delivery {
	if d, ok := deliveries[platformKey{runtime.GOOS, runtime.GOARCH}]; ok {
		return d
	}
	return fallbackDelivery{}
}
