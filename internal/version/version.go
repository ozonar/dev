package version

// Version текущая версия программы dev.
// Может быть переопределена при сборке через -ldflags "-X dev/internal/version.Version=..."
var Version = "1.0.3"
