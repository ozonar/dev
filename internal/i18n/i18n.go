// Пакет i18n обеспечивает интернационализацию сообщений CLI-приложения.
//
// Принцип работы:
//   - Сообщения в коде пишутся на английском (это msgid).
//   - Словари переводов лежат в locales/*.json и встраиваются в бинарник
//     через go:embed.
//   - Активный язык определяется из окружения (LC_ALL, LC_MESSAGES, LANG)
//     или задаётся явно через SetLang (флаг --lang).
//   - Если перевода нет, возвращается исходный английский текст — приложение
//     никогда не ломается из-за неполного словаря.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
)

//go:embed locales/*.json
var localeFS embed.FS

// catalog — плоский словарь переводов текущей локали.
// Ключи обычных сообщений совпадают с msgid (английской строкой).
// Ключи множественных форм имеют вид "<msgid>.<form>", где form —
// one/few/many/other.
type catalog map[string]string

var (
	mu       sync.RWMutex
	catalogs = map[string]catalog{}
	// lang — разрешённый код языка ("en"/"ru"), пустая строка означает,
	// что язык ещё не определён (нужно смотреть окружение).
	lang string
	// explicit — был ли язык задан явно через SetLang.
	explicit bool
)

func init() {
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			continue
		}
		var c catalog
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}
		catalogs[name] = c
	}
}

// Supported возвращает список кодов доступных языков.
func Supported() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(catalogs))
	for code := range catalogs {
		out = append(out, code)
	}
	return out
}

// SetLang задаёт язык явно (например, из флага --lang или переменной
// окружения DEV_LANG). Значение "auto" (или пустая строка) означает
// определение языка из окружения. Неизвестный код приводит к "en".
func SetLang(code string) {
	mu.Lock()
	defer mu.Unlock()
	lang = normalize(strings.TrimSpace(code))
	explicit = lang != ""
}

// Current возвращает активный код языка ("en"/"ru").
func Current() string {
	mu.Lock()
	defer mu.Unlock()
	if lang == "" {
		lang = detectLang()
	}
	return lang
}

// normalize приводит код языка к двухбуквенному виду: "ru_RU.UTF-8" -> "ru".
// Возвращает "" для auto/пустого значения и "en" для неизвестных кодов.
func normalize(code string) string {
	if code == "" || strings.EqualFold(code, "auto") {
		return ""
	}
	lower := strings.ToLower(code)
	// Отбрасываем регион и кодировку: ru_RU.UTF-8, en-US@euro.
	if i := strings.IndexAny(lower, "_-."); i > 0 {
		lower = lower[:i]
	}
	if _, ok := catalogs[lower]; !ok {
		return "en"
	}
	return lower
}

// detectLang определяет язык из переменных окружения в порядке приоритета:
// LC_ALL > LC_MESSAGES > LANG. По умолчанию — английский.
func detectLang() string {
	if explicit && lang != "" {
		return lang
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			if l := normalize(v); l != "" {
				return l
			}
		}
	}
	return "en"
}

// T переводит сообщение msgid на активный язык и, если переданы аргументы,
// подставляет их в получившийся формат. Аргументы сохраняют позиции
// format-вербов (%s, %d, %v), поэтому перевод может менять порядок слов.
func T(msgid string, args ...interface{}) string {
	s := lookup(msgid)
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// N переводит сообщение с учётом правил множественного числа активного
// языка. msgid — английская форма единственного числа; в словаре формы
// хранятся под ключами "<msgid>.one/.few/.many/.other".
// Если нужная форма отсутствует, используется msgid как есть.
// Число n всегда подставляется в формат первым аргументом; args идут следом.
func N(msgid string, n int, args ...interface{}) string {
	form := pluralForm(n)
	key := msgid + "." + form
	s, ok := lookupOK(key)
	if !ok {
		// Падение на форму other (для en) или на исходный ключ.
		if other, ok2 := lookupOK(msgid + ".other"); ok2 {
			s = other
		} else {
			s = msgid
		}
	}
	return fmt.Sprintf(s, append([]interface{}{n}, args...)...)
}

// pluralForm возвращает грамматическую форму для числа n.
// Русский: 1,21,31 -> one; 2-4,22-34 -> few; 0,5-20,11-14 -> many.
// Английский: 1 -> one, остальное -> other.
func pluralForm(n int) string {
	switch Current() {
	case "ru":
		a := n
		if a < 0 {
			a = -a
		}
		last := a % 10
		lastTwo := a % 100
		switch {
		case last == 1 && lastTwo != 11:
			return "one"
		case last >= 2 && last <= 4 && (lastTwo < 12 || lastTwo > 14):
			return "few"
		default:
			return "many"
		}
	default:
		if n == 1 {
			return "one"
		}
		return "other"
	}
}

// lookup возвращает перевод msgid или сам msgid, если перевода нет.
func lookup(msgid string) string {
	s, _ := lookupOK(msgid)
	return s
}

// lookupOK возвращает перевод и признак его наличия.
func lookupOK(msgid string) (string, bool) {
	l := Current()
	if l == "en" {
		return msgid, false
	}
	mu.RLock()
	c, ok := catalogs[l]
	mu.RUnlock()
	if !ok {
		return msgid, false
	}
	if v, ok := c[msgid]; ok {
		return v, true
	}
	// Канонизация ключа: словарь хранит строки без завершающего перевода
	// строки, поэтому для msgid с хвостовым "\n" ищем базовый ключ
	// и восстанавливаем перенос в переводе.
	if strings.HasSuffix(msgid, "\n") {
		if v, ok := c[strings.TrimSuffix(msgid, "\n")]; ok {
			return v + "\n", true
		}
	}
	return msgid, false
}

// --- Цветные обёртки ---
// Каждая функция переводит форматную строку и печатает её соответствующим
// цветом. Сигнатуры повторяют fatih/color, что упрощает миграцию кода.

// Red печатает переведённое сообщение красным.
func Red(format string, a ...interface{}) { color.Red(noArgs(format, a...)) }

// Green печатает переведённое сообщение зелёным.
func Green(format string, a ...interface{}) { color.Green(noArgs(format, a...)) }

// Yellow печатает переведённое сообщение жёлтым.
func Yellow(format string, a ...interface{}) { color.Yellow(noArgs(format, a...)) }

// Cyan печатает переведённое сообщение голубым.
func Cyan(format string, a ...interface{}) { color.Cyan(noArgs(format, a...)) }

// White печатает переведённое сообщение белым.
func White(format string, a ...interface{}) { color.White(noArgs(format, a...)) }

// Blue печатает переведённое сообщение синим.
func Blue(format string, a ...interface{}) { color.Blue(noArgs(format, a...)) }

// Magenta печатает переведённое сообщение пурпурным.
func Magenta(format string, a ...interface{}) { color.Magenta(noArgs(format, a...)) }

// HiRed печатает переведённое сообщение ярко-красным.
func HiRed(format string, a ...interface{}) { color.HiRed(noArgs(format, a...)) }

// HiGreen печатает переведённое сообщение ярко-зелёным.
func HiGreen(format string, a ...interface{}) { color.HiGreen(noArgs(format, a...)) }

// HiYellow печатает переведённое сообщение ярко-жёлтым.
func HiYellow(format string, a ...interface{}) { color.HiYellow(noArgs(format, a...)) }

// HiCyan печатает переведённое сообщение ярко-голубым.
func HiCyan(format string, a ...interface{}) { color.HiCyan(noArgs(format, a...)) }

// HiWhite печатает переведённое сообщение ярко-белым.
func HiWhite(format string, a ...interface{}) { color.HiWhite(noArgs(format, a...)) }

// HiBlue печатает переведённое сообщение ярко-синим.
func HiBlue(format string, a ...interface{}) { color.HiBlue(noArgs(format, a...)) }

// HiMagenta печатает переведённое сообщение ярко-пурпурным.
func HiMagenta(format string, a ...interface{}) { color.HiMagenta(noArgs(format, a...)) }

// noArgs переводит формат и подставляет аргументы, чтобы дальше можно было
// вызвать цветную функцию без аргументов.
func noArgs(format string, a ...interface{}) string {
	return T(format, a...)
}

// --- Обёртки fmt ---

// Printf печатает переведённое сообщение с подстановкой аргументов.
func Printf(format string, a ...interface{}) { fmt.Printf(T(format, a...)) }
