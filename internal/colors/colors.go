package colors

// ANSI escape codes for colors
const (
	Reset        = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorGray    = "\033[90m"
)

// Colorize возвращает строку с применённым цветом
func Colorize(text string, color string) string {
	return color + text + Reset
}

// Red возвращает строку красного цвета
func Red(text string) string {
	return Colorize(text, ColorRed)
}

// Green возвращает строку зелёного цвета
func Green(text string) string {
	return Colorize(text, ColorGreen)
}

// Yellow возвращает строку жёлтого цвета
func Yellow(text string) string {
	return Colorize(text, ColorYellow)
}

// Blue возвращает строку синего цвета
func Blue(text string) string {
	return Colorize(text, ColorBlue)
}

// Cyan возвращает строку голубого цвета
func Cyan(text string) string {
	return Colorize(text, ColorCyan)
}

// Magenta возвращает строку пурпурного цвета
func Magenta(text string) string {
	return Colorize(text, ColorMagenta)
}

// White возвращает строку белого цвета
func White(text string) string {
	return Colorize(text, ColorWhite)
}

// Gray возвращает строку серого цвета
func Gray(text string) string {
	return Colorize(text, ColorGray)
}
