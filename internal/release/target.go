// Целевая модель команд релиза: операция может применяться к одному
// релизу или ко всей группе. Группы не хранятся отдельно — они выводятся
// из свойства groups каждого релиза, поэтому источник истины единственный.
package release

import (
	"fmt"
	"sort"
	"strings"
)

// TargetKind — разновидность цели операции.
type TargetKind int

const (
	// TargetRelease — одиночный релиз из раздела releases.
	TargetRelease TargetKind = iota
	// TargetGroup — группа релизов (объединение по свойству groups).
	TargetGroup
)

// Target — цель операции релиза: либо один релиз, либо группа целиком.
// Kind задаёт смысл поля Name, поэтому состояние с пустым/чужим Name
// для выбранного Kind не представимо без ошибки резолва.
type Target struct {
	Kind TargetKind
	Name string
}

// GroupNames возвращает имена всех групп, встречающихся у релизов,
// отсортированные по алфавиту для стабильного показа.
func (c *Config) GroupNames() []string {
	seen := make(map[string]bool)
	for _, rel := range c.Releases {
		for _, g := range rel.Groups {
			seen[g] = true
		}
	}
	names := make([]string, 0, len(seen))
	for g := range seen {
		names = append(names, g)
	}
	sort.Strings(names)
	return names
}

// GroupMembers возвращает имена релизов, входящих в группу, в алфавитном
// порядке. Порядок важен для детерминированного деплоя всей группы.
func (c *Config) GroupMembers(groupName string) ([]string, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, fmt.Errorf("empty group name")
	}
	members := make([]string, 0)
	for name, rel := range c.Releases {
		for _, g := range rel.Groups {
			if g == groupName {
				members = append(members, name)
				break
			}
		}
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("unknown group %q", groupName)
	}
	sort.Strings(members)
	return members, nil
}

// GroupsOf возвращает группы конкретного релиза (свойство groups),
// отсортированные по алфавиту. Для неизвестного релиза — nil.
func (c *Config) GroupsOf(releaseName string) []string {
	rel := c.Releases[releaseName]
	if rel == nil {
		return nil
	}
	out := append([]string(nil), rel.Groups...)
	sort.Strings(out)
	return out
}

// ResolveTarget превращает имя из аргумента командной строки в цель.
// При совпадении имён релиза и группы приоритет у релиза — так имя
// релиза никогда не маскируется одноимённой группой.
func (c *Config) ResolveTarget(arg string) (Target, error) {
	if strings.TrimSpace(arg) == "" {
		return Target{}, fmt.Errorf("empty target name")
	}
	if c.Releases[arg] != nil {
		return Target{Kind: TargetRelease, Name: arg}, nil
	}
	if _, err := c.GroupMembers(arg); err == nil {
		return Target{Kind: TargetGroup, Name: arg}, nil
	}
	return Target{}, fmt.Errorf("unknown release or group %q", arg)
}

// AllTargets возвращает все возможные цели в порядке показа меню:
// сначала группы, затем релизы (внутри сегмента — по алфавиту).
func (c *Config) AllTargets() []Target {
	names := c.GroupNames()
	out := make([]Target, 0, len(names)+len(c.Releases))
	for _, g := range names {
		out = append(out, Target{Kind: TargetGroup, Name: g})
	}
	releaseNames := make([]string, 0, len(c.Releases))
	for name := range c.Releases {
		releaseNames = append(releaseNames, name)
	}
	sort.Strings(releaseNames)
	for _, n := range releaseNames {
		out = append(out, Target{Kind: TargetRelease, Name: n})
	}
	return out
}

// Important сообщает, помечен ли релиз флагом important.
func (c *Config) Important(releaseName string) bool {
	rel := c.Releases[releaseName]
	return rel != nil && rel.Important
}

// NeedsConfirmation сообщает, требует ли переключение на указанную цель
// явного подтверждения: цель — важный релиз, либо группа, содержащая
// хотя бы один важный релиз. Подтверждение спрашивает именно шаг switch.
func (c *Config) NeedsConfirmation(t Target) bool {
	switch t.Kind {
	case TargetRelease:
		return c.Important(t.Name)
	case TargetGroup:
		members, err := c.GroupMembers(t.Name)
		if err != nil {
			return false
		}
		for _, m := range members {
			if c.Important(m) {
				return true
			}
		}
	}
	return false
}

// TargetLabel формирует человекочитаемое описание цели для вывода.
func (c *Config) TargetLabel(t Target) string {
	switch t.Kind {
	case TargetGroup:
		members, err := c.GroupMembers(t.Name)
		if err == nil {
			return fmt.Sprintf("group %q (%s)", t.Name, strings.Join(members, ", "))
		}
		return fmt.Sprintf("group %q", t.Name)
	case TargetRelease:
		if groups := c.GroupsOf(t.Name); len(groups) > 0 {
			return fmt.Sprintf("release %q (groups: %s)", t.Name, strings.Join(groups, ", "))
		}
		return fmt.Sprintf("release %q", t.Name)
	}
	return t.Name
}
