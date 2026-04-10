package version

// Version текущая версия программы dev.
// Может быть переопределена при сборке через -ldflags "-X dev/internal/version.Version=..."
var Version = "local-build"
