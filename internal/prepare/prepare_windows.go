//go:build windows

package prepare

// PrepareProject — заглушка для Windows.
// На Windows подготовка проекта не требуется.
func PrepareProject(framework, language string) error {
	return nil
}
