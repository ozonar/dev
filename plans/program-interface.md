# Дизайн: Executable (интерфейс) + Program + Runtime

## Модель
- **`Executable` — интерфейс.** Всё, что можно скачать и запустить. Общая логика
  скачивания работает только с ним.
- **`Program`** — реализация для программ (lnav, линтеры). Версия фиксированная,
  версие-логики нет.
- **`Runtime`** — реализация для рантаймов (php, go). Инкапсулирует ВСЁ
  версие-зависимое: путь к бинарю для версии, сравнение версий, поиск системного,
  список доступных кандидатов, выбор подходящей версии.

Правила:
- скачиваем → через `Executable` (логика одинакова);
- работаем с рантаймом → используем `Runtime` (он сам находит и выбирает версию);
- работаем с программой → используем `Program`.

## Executable
```go
type Executable interface {
    Name() string
    Version() string
    Binary() string      // путь к бинарю внутри <tools>/<name>/<version>
    URL() string
    Archive() string
    FullCommand() string
    Require() []Executable
}
```

## Program (реализует Executable)
```go
type Program struct {
    name, version, binary, url, archive, fullCommand string
    require []Executable
}
// геттеры под интерфейс
```

## Runtime (реализует Executable + вся версие-логика)
```go
type Runtime struct {
    name, fullCommand string
    binaryFor func(version string) string   // usr/bin/php8.5
    satisfies func(installed, required string) bool
    systemBin, systemVer string
    version, binary, url, archive string
}

// --- Executable ---
func (rt *Runtime) Name() string        { return rt.name }
func (rt *Runtime) Version() string     { return rt.version }
func (rt *Runtime) Binary() string      { return rt.binary }
func (rt *Runtime) FullCommand() string { return rt.fullCommand }
func (rt *Runtime) URL() string         { return rt.url }
func (rt *Runtime) Archive() string     { return rt.archive }
func (rt *Runtime) Require() []Executable { return nil }

// WithVersion — версия с корректным Binary (решает баг с php8.5).
func (rt *Runtime) WithVersion(v string) *Runtime {
    c := *rt
    c.version = v
    c.binary = rt.binaryFor(v)
    return &c
}

// Satisfies — подходит ли installed под required (без switch в Manager:
// каждый Runtime хранит свой предикат).
func (rt *Runtime) Satisfies(installed, required string) bool {
    return rt.satisfies(installed, required)
}

// System — имя бинаря в PATH и команда определения версии.
func (rt *Runtime) System() (bin, ver string, ok bool) {
    return rt.systemBin, rt.systemVer, rt.systemBin != ""
}

// Available собирает кандидатов: системный рантайм (если есть) и версии,
// скачанные в папке dir. Возвращает готовые к выбору Executable.
func (rt *Runtime) Available(dir string) []Executable {
    var out []Executable
    if sys, ok := systemRuntime(rt, dir); ok {
        out = append(out, sys)
    }
    // ... проход по папкам версий: rt.WithVersion(e.Name()) ...
    return out
}

// Resolve выбирает из доступных кандидатов подходящий под требование required.
func (rt *Runtime) Resolve(dir, required string) (Executable, bool) {
    for _, cand := range rt.Available(dir) {
        if !rt.Satisfies(cand.Version(), required) {
            continue
        }
        return cand, true
    }
    return nil, false
}
```

Весь switch из старого `resolveAvailable` (php→atLeast, default→exact) ушёл внутрь
`Runtime.satisfies`. Manager больше не различает языки.

## Manager
- `Ensure(Executable...)` / `download` / `Command` — через интерфейс.
- `resolve` — если `Executable` является `*Runtime`, зовёт `rt.Resolve(m.dir, req)`;
  если сетевой резолюции не было — качает через интерфейс.

## Места правок
- `internal/toolchain`: `executable.go` (интерфейс), `program.go`, `runtime.go`,
  Manager (Ensure/download/resolve/systemProgram), фабрики php/go/lnav/линтеров.
- `internal/check`, `internal/run`, `internal/build`: переход на `Executable`.



это черновик. а не финальная версия. требования к финальной версии нужно будет выяснять на месте