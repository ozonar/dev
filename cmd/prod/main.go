package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dev/internal/ai"
	"dev/internal/colors"
	"dev/internal/install"
	"dev/internal/prod"
	"dev/internal/release"
	"dev/internal/update"
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

	The target can be a single release or a whole group (releases that list the
	group name in their "groups" property). For a group, every member release is
	processed in order. Releases marked important ask for confirmation before
	the switch step.

	Commands:
	  prepare [name]   copy build artifacts to releases/release-<datetime>
	  switch [name]    switch the current release symlink
	  release          prepare then switch (both steps in order)

Examples:
  prod release prepare backend
  prod release switch -l 5
  prod release web    # release the whole group "web"
  prod release`,
	Run: func(cmd *cobra.Command, args []string) {
		runReleaseBoth()
	},
}

var releasePrepareCmd = &cobra.Command{
	Use:   "prepare [name]",
	Short: "Copy build artifacts into a new release folder",
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

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update prod to the latest version",
	Long: `Download the latest prod binary from GitHub releases and install it.
The binary is downloaded to the home directory, installed via 'prod install',
and then the temporary file is removed.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := update.SelfUpdate("prod"); err != nil {
			fmt.Println(colors.Red("Update failed: " + err.Error()))
		}
	},
}

var installCmd = &cobra.Command{
	Use:   "install [file]",
	Short: "Install prod (or specified file) to system",
	Long: `Install copies the prod executable (or a specified file) to a system directory.
If no file argument is provided, installs the currently running prod binary.
You will be prompted to choose installation directory: /usr/local/bin (default) or ~/bin.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var file string
		if len(args) > 0 {
			file = args[0]
		}
		runInstall(file)
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
	rootCmd.AddCommand(selfUpdateCmd)
	rootCmd.AddCommand(installCmd)

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

// runInstall выполняет команду install: копирует указанный файл (или текущий
// бинарник) в выбранную системную директорию.
func runInstall(file string) {
	fmt.Println(colors.Cyan("Installing prod..."))
	if err := install.Install(file); err != nil {
		fmt.Println(colors.Red("Install failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Installation successful."))
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

// selectTarget определяет цель операции из аргумента либо интерактивно.
// Меню показывает цели по группам: сначала группы (со списком входящих
// релизов), затем одиночные релизы. Важные цели помечаются меткой.
func selectTarget(cfg *release.Config, arg string) (release.Target, error) {
	if arg != "" {
		return cfg.ResolveTarget(arg)
	}
	targets := cfg.AllTargets()
	fmt.Println(colors.Cyan("Available targets:"))
	for i, t := range targets {
		var label string
		switch t.Kind {
		case release.TargetGroup:
			members, err := cfg.GroupMembers(t.Name)
			if err != nil {
				return release.Target{}, err
			}
			label = "Group: " + t.Name + " [" + strings.Join(members, ", ") + "]"
		default:
			label = "Release: " + t.Name
		}
		if cfg.NeedsConfirmation(t) {
			label += " " + colors.Yellow("[important]")
		}
		fmt.Printf("  %d. %s\n", i+1, label)
	}
	idx, err := release.SelectIndex(os.Stdin, os.Stdout, "Select target", len(targets))
	if err != nil {
		return release.Target{}, err
	}
	return targets[idx], nil
}

// confirmImportant спрашивает подтверждение, если цель помечена как важная
// (important релиз или группа с важными релизами). Возвращает false, когда
// пользователь отменил операцию. Подтверждение относится к шагу switch.
func confirmImportant(cfg *release.Config, t release.Target) bool {
	if !cfg.NeedsConfirmation(t) {
		return true
	}
	fmt.Println(colors.Yellow("Important target selected: " + cfg.TargetLabel(t)))
	fmt.Print("Are you really sure you want to switch? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// runReleasePrepare выполняет команду prod release prepare [target]:
// копирует содержимое builds_folder в releases_folder/release-<datetime>,
// оставляя папку сборки нетронутой. Для группы готовятся все её релизы.
func runReleasePrepare(args []string) {
	cfg, err := release.EnsureConfig(".")
	if err != nil {
		fmt.Println(colors.Red("release config error: " + err.Error()))
		return
	}
	target, err := selectTarget(cfg, argOrEmpty(args))
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	if target.Kind == release.TargetGroup {
		prepared, err := cfg.PrepareGroup(target.Name, time.Now())
		if err != nil {
			fmt.Println(colors.Red("prepare failed: " + err.Error()))
			return
		}
		for _, p := range prepared {
			fmt.Println(colors.Green("Release prepared: " + p.Release + " -> " + p.Name))
		}
		return
	}
	created, err := release.Prepare(cfg.Releases[target.Name], time.Now())
	if err != nil {
		fmt.Println(colors.Red("prepare failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Release prepared: " + target.Name + " -> " + created))
}

// switchOneRelease переключает один релиз на выбранную папку с выводом
// результата. Возвращает ошибку, если переключение не удалось.
func switchOneRelease(rel *release.Release, releaseName string) error {
	if err := release.SwitchRelease(rel, releaseName); err != nil {
		return err
	}
	fmt.Println(colors.Green("Switched " + rel.CurrentReleaseLink + " -> " + filepath.Join(rel.ReleasesFolder, releaseName)))
	return nil
}

// switchGroup переключает каждый релиз группы на его самый свежий
// подготовленный релиз. Группа без подготовленных релизов пропускается.
func switchGroup(cfg *release.Config, groupName string) error {
	members, err := cfg.GroupMembers(groupName)
	if err != nil {
		return err
	}
	for _, name := range members {
		rel := cfg.Releases[name]
		infos, err := release.ListReleases(rel)
		if err != nil {
			return fmt.Errorf("release %q: %w", name, err)
		}
		if len(infos) == 0 {
			fmt.Println(colors.Yellow("No releases found in " + rel.ReleasesFolder + " (" + name + ")"))
			continue
		}
		if err := switchOneRelease(rel, infos[0].Name); err != nil {
			return fmt.Errorf("release %q: %w", name, err)
		}
	}
	return nil
}

// runReleaseSwitch выполняет команду prod release switch [target]: показывает
// список последних релизов (сегодняшние подсвечены белым фоном) и переключает
// симлинк current_release_folder на выбранный. Для группы переключаются все
// её релизы на самые свежие версии. Важная цель требует подтверждения.
func runReleaseSwitch(args []string) {
	cfg, err := release.EnsureConfig(".")
	if err != nil {
		fmt.Println(colors.Red("release config error: " + err.Error()))
		return
	}
	target, err := selectTarget(cfg, argOrEmpty(args))
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	// Переспрос перед сменой симлинка на важный релиз или группу.
	if !confirmImportant(cfg, target) {
		fmt.Println(colors.Yellow("Switch cancelled."))
		return
	}

	if target.Kind == release.TargetGroup {
		if err := switchGroup(cfg, target.Name); err != nil {
			fmt.Println(colors.Red("switch failed: " + err.Error()))
		}
		return
	}

	rel := cfg.Releases[target.Name]
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
	fmt.Println(colors.Cyan("Recent releases (" + target.Name + "):"))
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
	if err := switchOneRelease(rel, recent[idx].Name); err != nil {
		fmt.Println(colors.Red("switch failed: " + err.Error()))
	}
}

// runReleaseBoth выполняет обе операции по порядку: prepare затем switch
// на только что созданные релизы. Для группы обрабатываются все её релизы,
// а важная цель требует подтверждения перед переключением.
func runReleaseBoth() {
	cfg, err := release.EnsureConfig(".")
	if err != nil {
		fmt.Println(colors.Red("release config error: " + err.Error()))
		return
	}
	target, err := selectTarget(cfg, "")
	if err != nil {
		fmt.Println(colors.Red(err.Error()))
		return
	}
	// Переспрос относится к шагу switch, поэтому спрашиваем до prepare:
	// при отмене не создаём лишних папок.
	if !confirmImportant(cfg, target) {
		fmt.Println(colors.Yellow("Release cancelled."))
		return
	}

	if target.Kind == release.TargetGroup {
		prepared, err := cfg.PrepareGroup(target.Name, time.Now())
		if err != nil {
			fmt.Println(colors.Red("prepare failed: " + err.Error()))
			return
		}
		for _, p := range prepared {
			if err := switchOneRelease(cfg.Releases[p.Release], p.Name); err != nil {
				fmt.Println(colors.Red("switch failed: " + err.Error()))
				return
			}
		}
		return
	}

	rel := cfg.Releases[target.Name]
	created, err := release.Prepare(rel, time.Now())
	if err != nil {
		fmt.Println(colors.Red("prepare failed: " + err.Error()))
		return
	}
	fmt.Println(colors.Green("Release prepared: " + created))
	if err := switchOneRelease(rel, created); err != nil {
		fmt.Println(colors.Red("switch failed: " + err.Error()))
	}
}
