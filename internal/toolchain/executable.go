package toolchain

// Executable описывает инструмент, который можно скачать и запустить:
// рантайм (php, go) или программу (lnav, линтер).
type Executable interface {
	// Name — короткое имя, используется в плейсхолдерах команд (например
	// {php}, {phpstan}).
	Name() string
	// Version — требуемая/актуальная версия.
	Version() string
	// Binary — относительный путь к исполняемому файлу внутри папки версии
	// <tools>/<name>/<version>, либо абсолютный путь для системного рантайма.
	Binary() string
	// URL — адрес скачивания. Для программ с динамически определяемым URL
	// (php, go) заполняется только при сетевой резолюции.
	URL() string
	// Archive — тип архива: "" (простой файл), "tar.gz", "tar.xz", "zip".
	Archive() string
	// FullCommand — команда для запуска с плейсхолдерами {имя}.
	FullCommand() string
	// Require — зависимости, которые должны быть доступны до запуска.
	Require() []Executable
}
