package recipe

import (
	"errors"
	"io"
	"os"
	"strings"

	"dev/internal/custom"
	"dev/internal/detector"
	"dev/internal/i18n"
)

// Run выполняет сценарий команды dev recipe.
// Без аргументов выводит список рецептов из свежего repository.yml.
// С именем — готовит рецепт (из кэша или репозитория), показывает описание,
// спрашивает подтверждение и выполняет его в текущей директории.
func Run(args []string, force bool) error {
	cwd, _ := os.Getwd()

	if len(args) == 0 {
		items, err := List()
		if err != nil {
			return err
		}
		printList(items)
		return nil
	}

	prepared, err := Prepare(args[0], force)
	if err != nil {
		var notFound *NotFoundError
		if errors.As(err, &notFound) {
			printNotFound(notFound)
			return nil
		}
		return err
	}

	printPrepared(prepared)
	if !confirm(os.Stdin) {
		i18n.Yellow("Skipped.")
		return nil
	}
	return execute(prepared, cwd, args[1:])
}

// printNotFound выводит отсутствующий рецепт и совпадения по подстроке.
func printNotFound(notFound *NotFoundError) {
	i18n.Red("Recipe %q not found.", notFound.Query)
	if len(notFound.Matches) > 0 {
		i18n.Yellow("Matching recipes:")
		printItems(notFound.Matches)
		return
	}
	i18n.Yellow("No recipes match %q.", notFound.Query)
}

// printPrepared выводит имя, описание и время попадания в кэш.
func printPrepared(prepared *Prepared) {
	i18n.Cyan("Recipe: %s", prepared.Name)
	if prepared.Description != "" {
		i18n.White("Description: %s", prepared.Description)
	}
	source := i18n.T("repository")
	if prepared.FromCache {
		source = i18n.T("cache")
	}
	i18n.White("Cached at: %s (%s)", prepared.CachedAt.Format("2006-01-02 15:04:05"), source)
}

// confirm спрашивает подтверждение запуска. Пустой ввод и "y"/"yes"
// означают запуск, поэтому по умолчанию рецепт выполняется.
func confirm(in io.Reader) bool {
	i18n.Printf("Run? [Y/n]: ")
	input := strings.TrimSpace(strings.ToLower(readLine(in)))
	return input == "" || input == "y" || input == "yes"
}

// readLine читает одну строку посимвольно, не буферизуя последующий ввод:
// интерактивный рецепт должен получить свой stdin целиком.
func readLine(in io.Reader) string {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	return b.String()
}

// execute выполняет рецепт в контексте текущего проекта.
// Детекция нужна только для подстановок $(language) и $(framework).
// params передаются рецепту как позиционные аргументы $1..$N.
func execute(prepared *Prepared, cwd string, params []string) error {
	ctx := custom.Context{Dir: cwd, Args: params}
	if info, err := detector.DetectProject(cwd); err == nil {
		ctx.Language = info.Language
		ctx.Framework = info.Framework
	}
	return Execute(prepared, ctx)
}

// printList выводит заголовок и список рецептов.
func printList(items []ListItem) {
	if len(items) == 0 {
		i18n.Yellow("No recipes found.")
		return
	}
	i18n.Cyan("Available recipes:")
	printItems(items)
}

// printItems выводит имена и описания рецептов без заголовка.
func printItems(items []ListItem) {
	for _, item := range items {
		if item.Description != "" {
			i18n.Printf("  %-20s %s\n", item.Name, item.Description)
		} else {
			i18n.Printf("  %s\n", item.Name)
		}
	}
}
