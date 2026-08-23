package toolchain

import (
	"os"
	"path/filepath"
)

// Runtime описывает версие-зависимый рантайм (php, go).
type Runtime interface {
	Executable

	// Satisfies определяет, что установленная версия подходит под требование.
	Satisfies(installed, required string) bool
	// System возвращает имя бинаря в PATH и команду определения его версии
	// (ok=false, если системного аналога нет).
	System() (bin, versionCmd string, ok bool)
	// IsSystem сообщает, что экземпляр является системным рантаймом из PATH.
	IsSystem() bool
	// Resolve выбирает из доступных кандидатов (системный + скачанные в dir)
	// первый, чья версия удовлетворяет требованию required. Сети не касается.
	Resolve(dir, required string) (Executable, bool)
	// ResolveDownload определяет URL для скачивания требуемой версии.
	// Вызывается только при реальном скачивании.
	ResolveDownload() (Runtime, error)
}

// runtimeBase — общая реализация Runtime для php и go. Конкретные классы Php
// и Go встраивают её и задают свой конфиг (путь к бинарю, сравнение версий,
// системный бинарь). Новые экземпляры создаются через фабрику runtimes, поэтому
// конкретный тип сохраняется без копирования.
type runtimeBase struct {
	name        string
	fullCommand string
	systemBin   string
	systemVer   string

	// Состояние конкретной версии.
	version  string
	binary   string
	url      string
	archive  string
	isSystem bool
}

// --- Executable ---

func (rt *runtimeBase) Name() string          { return rt.name }
func (rt *runtimeBase) Version() string       { return rt.version }
func (rt *runtimeBase) Binary() string        { return rt.binary }
func (rt *runtimeBase) URL() string           { return rt.url }
func (rt *runtimeBase) Archive() string       { return rt.archive }
func (rt *runtimeBase) FullCommand() string   { return rt.fullCommand }
func (rt *runtimeBase) Require() []Executable { return nil }

// System возвращает имя бинаря в PATH и команду определения его версии.
func (rt *runtimeBase) System() (bin, versionCmd string, ok bool) {
	return rt.systemBin, rt.systemVer, rt.systemBin != ""
}

// IsSystem сообщает, что экземпляр является системным рантаймом из PATH.
func (rt *runtimeBase) IsSystem() bool {
	return rt.isSystem
}

// Resolve выбирает из доступных кандидатов подходящий под требование required.
// Кандидаты создаются фабрикой runtimes как конкретные типы (Go, Php), поэтому
// сравнение версий выполняется их собственным методом Satisfies.
func (rt *runtimeBase) Resolve(dir, required string) (Executable, bool) {
	for _, cand := range rt.Available(dir) {
		r, ok := cand.(Runtime)
		if !ok || !r.Satisfies(cand.Version(), required) {
			continue
		}
		return cand, true
	}
	return nil, false
}

// Available собирает кандидатов без сети: системный рантайм (если есть) и
// версии, уже скачанные в папку dir.
func (rt *runtimeBase) Available(dir string) []Executable {
	var out []Executable
	if sys, ok := getSystemRuntime(rt); ok {
		out = append(out, sys)
	}
	out = append(out, rt.downloadedVersions(dir)...)
	return out
}

// downloadedVersions возвращает скачанные версии (<dir>/<name>/<version>).
// Каждая версия создаётся через фабрику runtimes, сохраняя конкретный тип.
func (rt *runtimeBase) downloadedVersions(dir string) []Executable {
	newRT, ok := runtimes[rt.name]
	if !ok {
		return nil
	}
	base := filepath.Join(dir, rt.name)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []Executable
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, newRT(e.Name()))
	}
	return out
}

// runtimes сопоставляет имя языка с фабрикой создания его Runtime.
var runtimes = map[string]func(version string) Runtime{
	"php": NewPhp,
	"go":  NewGo,
}

// runtimeFor возвращает фабрику Runtime по имени программы, если она есть.
func runtimeFor(name string) (func(version string) Runtime, bool) {
	newRT, ok := runtimes[name]
	return newRT, ok
}

// getSystemRuntime находит системный рантайм rt в PATH и определяет его версию.
// Возвращает экземпляр с абсолютным путём к бинарю и признаком isSystem=true.
func getSystemRuntime(rt *runtimeBase) (Runtime, bool) {
	bin, versionCmd, ok := rt.System()
	if !ok {
		return nil, false
	}
	path := lookupSystem(bin)
	if path == "" {
		return nil, false
	}
	version := runVersionCmd(versionCmd)
	if version == "" {
		return nil, false
	}
	sys := runtimes[rt.name](version)
	switch s := sys.(type) {
	case *Php:
		s.markSystem(path)
	case *Go:
		s.markSystem(path)
	default:
		return nil, false
	}
	return sys, true
}

// PhpProgram возвращает описание php-рантайма требуемой версии без сети.
func PhpProgram(version string) Executable {
	return NewPhp(version)
}

// GoProgram возвращает описание go-рантайма требуемой версии без сети.
func GoProgram(version string) Executable {
	return NewGo(version)
}
