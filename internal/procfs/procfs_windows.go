//go:build windows

package procfs

import "errors"

// errUnsupported — на Windows нет ни /proc, ни lsof, поэтому инспекция
// процессов и сокетов не поддерживается.
var errUnsupported = errors.New("process inspection is not supported on windows")

// Listeners возвращает ошибку: на Windows определение владельцев порта
// недоступно.
func Listeners(port int) ([]Listener, error) {
	return nil, errUnsupported
}

// Holders возвращает ошибку: на Windows определение держателей файла
// недоступно.
func Holders(path string) ([]int, error) {
	return nil, errUnsupported
}
