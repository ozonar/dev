package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dev/internal/ai"
	"dev/internal/colors"
	"dev/internal/prod"
	"dev/internal/release"
	"dev/internal/virus"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "prod",
	Short: "Analyze production server health",
	Long: `prod — диагностика состояния продакшен-сервера.

Собирает данные о категориях (CPU, память, диск, БД, ...), определяет
симптомы, сохраняет отчёт в /etc/prod-command/reports/ и предлагает
дальнейший анализ: построение причинной цепочки или LLM-отчёт.`,
	Run: func(cmd *cobra.Command, args []string) {
		runBrief()
	},
}

// statAll — показывать полный отчёт по всем категориям (флаг --all).
var statAll bool

var statCmd = &cobra.Command{
	Use:     "stat [category]",
	Aliases: []string{"status"},
	Short:   "Show brief report or a single category report",
	Long: `Shows a brief health report. If a category is given (cpu, memory, disk,
fd, network, php-fpm, pgsql, redis, external, recent), shows the detailed
report for that category only.

Flags:
  --all, -a   show the full report with all categories

Examples:
  prod            # brief report + analysis choice
  prod stat       # same as prod
  prod stat --all # full report, all categories
  prod stat cpu   # detailed CPU report
  prod stat pgsql # detailed PostgreSQL report`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if statAll {
			runDetail()
			return
		}
		if len(args) == 1 {
			runCategory(args[0])
			return
		}
		runBrief()
	},
}

var detailCmd = &cobra.Command{
	Use:     "detail",
	Aliases: []string{"details", "full"},
	Short:   "Show full report with all categories",
	Run: func(cmd *cobra.Command, args []string) {
		runDetail()
	},
}

var cascadeCmd = &cobra.Command{
	Use:     "cascading",
	Aliases: []string{"cascade", "chain"},
	Short:   "Build a causal chain (cascading failure)",
	Run: func(cmd *cobra.Command, args []string) {
		rep := collectAndSave()
		prod.RenderCascade(prod.BuildCascade(rep))
	},
}

var llmCmd = &cobra.Command{
	Use:     "llm",
	Aliases: []string{"report"},
	Short:   "Send collected data to LLM for analysis",
	Run: func(cmd *cobra.Command, args []string) {
		rep := collectAndSave()
		runLLM(rep)
	},
}

// releaseLines — количество последних релизов для показа в команде switch.
var releaseLines int

var releaseCmd = &cobra.Command{
	Use:     "release",
	Aliases: []string{"deploy"},
	Short:   "Prepare and switch production releases",
	Long: `Prepares a new release folder from build artifacts and switches the
active release via a symlink. Configuration is read from release.yml in the
current directory; if missing, an editor opens with a filled template.

Commands:
  prepare [name]   move build artifacts to releases/release-<datetime>
  switch [name]    switch the current release symlink
  release          prepare then switch (both steps in order)

Examples:
  prod release prepare backend
  prod release switch -l 5
  prod release`,
	Run: func(cmd *cobra.Command, args []string) {
		runReleaseBoth()
	},
}

var releasePrepareCmd = &cobra.Command{
	Use:   "prepare [name]",
	Short: "Move build artifacts into a new release folder",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runReleasePrepare(args)
	},
}

var releaseSwitchCmd = &cobra.Command{
	Use:   "switch [name]",
	Short: "Switch the current release symlink",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runReleaseSwitch(args)
	},
}

var virusCmd = &cobra.Command{
	Use:   "virus [user@ip_addr]",
	Short: "Copy itself to remote server",
	Long: `Copy the prod executable to a remote server via SCP and install the
production configuration there: /etc/prod-command (config files, e.g. deps.conf;
the reports history is not copied) and the LLM config, so reports, cascade
analysis and 'prod llm' work right away.

Format: user@ip or just ip (SSH key auth only, password is not supported).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runVirus(args[0])
	},
}

func main() {
	statCmd.Flags().BoolVarP(&statAll, "all", "a", false, "Show full report with all categories")
	releaseCmd.PersistentFlags().IntVarP(&releaseLines, "lines", "l", 5, "Number of recent releases to show")
	rootCmd.AddCommand(statCmd)
	rootCmd.AddCommand(detailCmd)
	rootCmd.AddCommand(cascadeCmd)
	rootCmd.AddCommand(llmCmd)
	releaseCmd.AddCommand(releasePrepareCmd)
	releaseCmd.AddCommand(releaseSwitchCmd)
	rootCmd.AddCommand(releaseCmd)
	rootCmd.AddCommand(virusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runVirus выполняет команду virus: копирует бинарник и прод-конфиги
// на удалённый сервер.
func runVirus(path string) {
	fmt.Println(colors.Cyan("Copying to remote server " + path + "..."))
	if err := virus.ProdVirus(path); err != nil {
		fmt.Println(colors.Red("Virus command failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Copy successful."))
}

// collectAndSave собирает отчёт (с учётом предыдущего снапшота) и сохраняет.
func collectAndSave() *prod.Report {
	prev, _ := prod.LoadPrevious(time.Now())
	rep := prod.Collect(prev)
	path, err := prod.SaveReport(rep)
	if err != nil {
		fmt.Println(colors.Yellow("report save skipped: " + err.Error()))
	} else {
		fmt.Println(colors.Gray("report saved to " + path))
	}
	return rep
}

// runBrief выводит краткий отчёт и открывает выбор дальнейших действий.
func runBrief() {
	rep := collectAndSave()
	fmt.Println()
	prod.RenderBrief(rep)
	offerAnalysis(rep)
}

// runDetail выводит подробный отчёт и открывает выбор.
func runDetail() {
	rep := collectAndSave()
	fmt.Println()
	prod.RenderDetail(rep)
	offerAnalysis(rep)
}

// offerAnalysis показывает меню выбора: cascading failure / llm report.
func offerAnalysis(rep *prod.Report) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println(colors.Gray("=============================================="))
	fmt.Println("Select next step:")
	fmt.Println("  1. Cascading failure (build causal chain)")
	fmt.Println("  2. LLM report (send data to AI)")
	fmt.Print(colors.Cyan("Select [1]: "))

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}
	switch line {
	case "2":
		runLLM(rep)
	default:
		prod.RenderCascade(prod.BuildCascade(rep))
	}
}

// runLLM отправляет отчёт в LLM.
func runLLM(rep *prod.Report) {
	cfg, err := ai.LoadConfig()
	if err != nil {
		fmt.Println(colors.Red("LLM config error: " + err.Error()))
		fmt.Println(colors.Yellow("Configure ~/dev-config/main.conf or /etc/dev-command/main.conf (LLM_ENDPOINT, LLM_TOKEN, LLM_MODEL)"))
		return
	}
	fmt.Println(colors.Cyan("Sending report to LLM (" + cfg.Model + ")..."))
	out, err := prod.GenerateLLMReport(prod.LLMOptions{
		Endpoint: cfg.Endpoint,
		Token:    cfg.Token,
		Model:    cfg.Model,
	}, rep)
	if err != nil {
		fmt.Println(colors.Red("LLM request failed: " + err.Error()))
		return
	}
	fmt.Println()
	fmt.Println(out)
}

// runCategory выводит подробный отчёт по одной категории.
func runCategory(name string) {
	rep := collectAndSave()
	id, ok := prod.CategoryByName(name)
	if !ok {
		fmt.Println(colors.Red("Unknown category \"" + name + "\""))
		fmt.Println(colors.Yellow("Available: " + prod.AvailableCategories()))
		return
	}
	cat := rep.Category(id)
	if cat == nil || !cat.Present {
		fmt.Println(colors.Yellow("Category " + id.Title() + " not detected on this host"))
		return
	}
	fmt.Println()
	prod.RenderCategory(cat)
}

// argOrEmpty возвращает первый позиционный аргумент или пустую строку.
func argOrEmpty(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// selectReleaseName определяет имя релиза из аргумента либо интерактивно:
// выводит список всех релизов конфига и просит выбрать номер (по умолчанию 1).
func selectReleaseName(cfg *release.Config, arg string) (string, error) {
	if arg != "" {
		if cfg.Releases[arg] == nil {
			return "", fmt.Errorf("unknown release %q", arg)
		}
		return arg, nil
	}
	names := make([]string, 0, len(cfg.Releases))
	for n := range cfg.Releases {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Println(colors.Cyan("Available releases:"))
	for i, n := range names {
		fmt.Printf("  %d. %s\n", i+1, n)
	}
	idx, err := release.SelectIndex(os.Stdin, os.Stdout, "Select release", len(names))
	if err != nil {
		return "", err
	}
	return names[idx], nil
}

// runReleasePrepare выполняет команду prod release prepare [name]:
// переносит содержимое builds_folder в releases_folder/release-<datetime>.
func runReleasePrepare(args []string) {
	cfg, err := release.EnsureConfig(".")
	if err != nil {
		fmt.Println(colors.Red("release config error: " + err.Error()))
		return
	}
	name, err := selectReleaseName(cfg, argOrEmpty(args))
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	created, err := release.Prepare(cfg.Releases[name], time.Now())
	if err != nil {
		fmt.Println(colors.Red("prepare failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Release prepared: " + name + " -> " + created))
}

// runReleaseSwitch выполняет команду prod release switch [name]: показывает
// список последних релизов (сегодняшние подсвечены белым фоном) и переключает
// симлинк current_release_folder на выбранный.
func runReleaseSwitch(args []string) {
	cfg, err := release.EnsureConfig(".")
	if err != nil {
		fmt.Println(colors.Red("release config error: " + err.Error()))
		return
	}
	name, err := selectReleaseName(cfg, argOrEmpty(args))
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	rel := cfg.Releases[name]

	infos, err := release.ListReleases(rel)
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	if len(infos) == 0 {
		fmt.Println(colors.Yellow("No releases found in " + rel.ReleasesFolder))
		return
	}

	limit := releaseLines
	if limit <= 0 {
		limit = 5
	}
	if limit > len(infos) {
		limit = len(infos)
	}
	recent := infos[:limit]

	// Текущий (связанный симлинком) релиз помечаем зелёным.
	current, hasCurrent := release.CurrentRelease(rel)
	// Сегодняшние релизы подсвечиваем белым задним фоном.
	todayStyle := color.New(color.BgWhite, color.FgBlack)
	fmt.Println(colors.Cyan("Recent releases (" + name + "):"))
	for i, r := range recent {
		label := r.Name
		if r.IsToday {
			label = todayStyle.Sprint(label)
		}
		if hasCurrent && r.Name == current {
			label += " " + colors.Green("(current)")
		}
		fmt.Printf("  %d. %s\n", i+1, label)
	}

	idx, err := release.SelectIndex(os.Stdin, os.Stdout, "Select release", len(recent))
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	target := recent[idx].Name
	if err := release.SwitchRelease(rel, target); err != nil {
		fmt.Println(colors.Red("switch failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Switched " + rel.CurrentReleaseLink + " -> " + filepath.Join(rel.ReleasesFolder, target)))
}

// runReleaseBoth выполняет обе операции по порядку: prepare затем switch
// на только что созданный релиз.
func runReleaseBoth() {
	cfg, err := release.EnsureConfig(".")
	if err != nil {
		fmt.Println(colors.Red("release config error: " + err.Error()))
		return
	}
	name, err := selectReleaseName(cfg, "")
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	rel := cfg.Releases[name]

	created, err := release.Prepare(rel, time.Now())
	if err != nil {
		fmt.Println(colors.Red("prepare failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Release prepared: " + created))

	if err := release.SwitchRelease(rel, created); err != nil {
		fmt.Println(colors.Red("switch failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Switched " + rel.CurrentReleaseLink + " -> " + filepath.Join(rel.ReleasesFolder, created)))
}
