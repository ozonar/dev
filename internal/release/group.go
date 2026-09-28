// Операции над группами релизов: подготовка нескольких релизов одним
// вызовом с детерминированным порядком из состава группы.
package release

import (
	"fmt"
	"time"
)

// Prepared — результат подготовки одного релиза внутри группы.
type Prepared struct {
	// Release — имя релиза из конфига (backend, frontend, ...).
	Release string
	// Name — имя созданной папки релиза (release-<datetime>).
	Name string
}

// PrepareGroup подготавливает все релизы группы в алфавитном порядке имён
// (порядок задаёт GroupMembers). При первой ошибке выполнение останавливается,
// уже созданные папки не удаляются — они перечислены в возвращаемом списке.
func (c *Config) PrepareGroup(groupName string, now time.Time) ([]Prepared, error) {
	members, err := c.GroupMembers(groupName)
	if err != nil {
		return nil, err
	}
	prepared := make([]Prepared, 0, len(members))
	for _, name := range members {
		created, err := Prepare(c.Releases[name], now)
		if err != nil {
			return prepared, fmt.Errorf("release %q: %w", name, err)
		}
		prepared = append(prepared, Prepared{Release: name, Name: created})
	}
	return prepared, nil
}
