package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"dev/internal/ai"
	"dev/internal/colors"
	"dev/internal/prod"

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

func main() {
	statCmd.Flags().BoolVarP(&statAll, "all", "a", false, "Show full report with all categories")
	rootCmd.AddCommand(statCmd)
	rootCmd.AddCommand(detailCmd)
	rootCmd.AddCommand(cascadeCmd)
	rootCmd.AddCommand(llmCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
