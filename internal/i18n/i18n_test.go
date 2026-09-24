package i18n

import (
	"testing"
)

// resetLang возвращает язык к автоопределению после теста.
func resetLang(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetLang("") })
}

func TestSupported(t *testing.T) {
	found := map[string]bool{}
	for _, l := range Supported() {
		found[l] = true
	}
	if !found["en"] {
		t.Error("Supported() не содержит en")
	}
	if !found["ru"] {
		t.Error("Supported() не содержит ru")
	}
}

func TestCurrentFromEnv(t *testing.T) {
	resetLang(t)
	// Без явной установки язык определяется из окружения и всегда валиден.
	lang := Current()
	if lang != "en" && lang != "ru" {
		t.Fatalf("Current() = %q, ожидался en или ru", lang)
	}
}

func TestSetLangNormalize(t *testing.T) {
	resetLang(t)
	cases := []struct {
		in   string
		want string
	}{
		{"ru", "ru"},
		{"ru_RU.UTF-8", "ru"},
		{"ru-RU@euro", "ru"},
		{"RU", "ru"},
		{"en", "en"},
		{"en_US", "en"},
		{"de", "en"}, // неизвестный язык — английский
	}
	for _, c := range cases {
		SetLang(c.in)
		if got := Current(); got != c.want {
			t.Errorf("SetLang(%q): Current() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTTranslation(t *testing.T) {
	SetLang("ru")
	resetLang(t)
	got := T("Error detecting project: %v", 42)
	want := "Ошибка определения проекта: 42"
	if got != want {
		t.Errorf("T() = %q, want %q", got, want)
	}
}

func TestTNoArgs(t *testing.T) {
	SetLang("ru")
	resetLang(t)
	if got := T("Build successful."); got != "Сборка выполнена успешно." {
		t.Errorf("T() = %q", got)
	}
}

func TestTFallback(t *testing.T) {
	SetLang("ru")
	resetLang(t)
	msg := "Some untranslated message %s"
	if got := T(msg, "x"); got != "Some untranslated message x" {
		t.Errorf("fallback T() = %q", got)
	}
}

func TestTEnglishIdentity(t *testing.T) {
	SetLang("en")
	resetLang(t)
	// Для английского перевод не ищется — msgid возвращается как есть.
	if got := T("Error detecting project: %v", 7); got != "Error detecting project: 7" {
		t.Errorf("en T() = %q", got)
	}
}

func TestNewlineCanonicalization(t *testing.T) {
	SetLang("ru")
	resetLang(t)
	// Словарь хранит канонические ключи без хвостового "\n":
	// msgid с "\n" обязан резолвиться в тот же перевод + перенос.
	got := T("Language:  %s\n", "PHP")
	want := "Язык:  PHP\n"
	if got != want {
		t.Errorf("T() = %q, want %q", got, want)
	}
}

func TestNPluralRU(t *testing.T) {
	SetLang("ru")
	resetLang(t)
	cases := []struct {
		n    int
		want string
	}{
		{1, "1 файл"},
		{2, "2 файла"},
		{5, "5 файлов"},
		{11, "11 файлов"},
		{21, "21 файл"},
		{22, "22 файла"},
		{25, "25 файлов"},
	}
	for _, c := range cases {
		if got := N("%d files", c.n); got != c.want {
			t.Errorf("N(ru, %d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestNPluralEN(t *testing.T) {
	SetLang("en")
	resetLang(t)
	// Для английского словаря нет — N возвращает msgid как есть,
	// сохраняя исходное поведение кода (без склонения).
	if got := N("%d files", 3); got != "3 files" {
		t.Errorf("N(en, 3) = %q", got)
	}
}

func TestNWithArgs(t *testing.T) {
	SetLang("ru")
	resetLang(t)
	// Аргументы подставляются в выбранную форму.
	if got := N("%d files", 4); got != "4 файла" {
		t.Errorf("N(ru, 4) = %q", got)
	}
}
