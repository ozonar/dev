package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"dev/internal/ai"
	"dev/internal/build"
	"dev/internal/cache"
	"dev/internal/check"
	"dev/internal/curl"
	"dev/internal/custom"
	"dev/internal/db"
	"dev/internal/debug"
	"dev/internal/detector"
	"dev/internal/docker"
	"dev/internal/install"
	"dev/internal/logs"
	"dev/internal/migrate"
	"dev/internal/port"
	"dev/internal/prepare"
	"dev/internal/run"
	"dev/internal/update"
	"dev/internal/version"
	"dev/internal/virus"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development assistant tool",
	Long:  "A CLI tool to analyze, manage, and run development projects.",
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze current project",
	Run: func(cmd *cobra.Command, args []string) {
		runAnalyze()
	},
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Clear framework cache",
	Run: func(cmd *cobra.Command, args []string) {
		runCache()
	},
}

var logsCmd = &cobra.Command{
	Use:     "logs",
	Aliases: []string{"log"},
	Short:   "Show logs and open in lnav",
	Run: func(cmd *cobra.Command, args []string) {
		runLogs()
	},
}

var runCmd = &cobra.Command{
	Use:     "run [port]",
	Aliases: []string{"start"},
	Short:   "Run the project",
	Long:    "Run the project's dev server. Optionally specify a port number as an argument or use --port flag.",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Если передан позиционный аргумент — используем его как порт
		if len(args) > 0 {
			p, err := strconv.Atoi(args[0])
			if err == nil {
				runPort = p
			}
		}
		runRun()
	},
}

var runPort int

// Флаги принудительного выбора языка для команд run/build/review.
// Позволяют запустить/собрать/проверить код указанного языка, игнорируя
// автоматическое определение проекта.
var forcedGo, forcedPHP, forcedJS, forcedPython bool

// forcedVersion — версия форсированного языка (флаг --version).
var forcedVersion string

func init() {
	runCmd.Flags().IntVarP(&runPort, "port", "p", 0, "Port for the dev server (default: 8000)")
	addLanguageFlags(runCmd, false)
	addLanguageFlags(buildCmd, false)
	addLanguageFlags(debugCmd, false)
	debugCmd.Flags().IntVarP(&debugPort, "port", "p", 0, "Port for the PHP debug server (default: 8000)")
}

// addLanguageFlags регистрирует флаги --go/--php/--js/--python на команде.
// Если persistent=true — флаги добавляются как PersistentFlags (наследуются
// подкомандами, например review fix / review ai).
func addLanguageFlags(cmd *cobra.Command, persistent bool) {
	f := cmd.Flags()
	if persistent {
		f = cmd.PersistentFlags()
	}
	f.BoolVar(&forcedGo, "go", false, "Force Go runtime (ignore detection)")
	f.BoolVar(&forcedPHP, "php", false, "Force PHP runtime (ignore detection)")
	f.BoolVar(&forcedJS, "js", false, "Force JavaScript/Node.js runtime (ignore detection)")
	f.BoolVar(&forcedPython, "python", false, "Force Python runtime (ignore detection)")
	f.StringVar(&forcedVersion, "version", "", "Force language version (e.g. 8.3 for PHP)")
}

// applyForcedLanguage переопределяет язык и framework проекта на основе флагов
// --go/--php/--js/--python (если они заданы), игнорируя результат детектора.
func applyForcedLanguage(info *detector.ProjectInfo) error {
	// Выбираем язык по флагам; конфликт нескольких флагов недопустим.
	lang, flag := "", ""
	set := func(name, value string) error {
		if lang != "" {
			return fmt.Errorf("conflicting language flags: --%s and --%s",
				flagName(flag), name)
		}
		lang, flag = value, name
		return nil
	}
	if forcedGo {
		if err := set("go", "go"); err != nil {
			return err
		}
	}
	if forcedPHP {
		if err := set("php", "php"); err != nil {
			return err
		}
	}
	if forcedJS {
		if err := set("js", "javascript"); err != nil {
			return err
		}
	}
	if forcedPython {
		if err := set("python", "python"); err != nil {
			return err
		}
	}

	// Версия из --version применяется к любому выбранному языку: и к
	// форсированному, и к детектированному (когда языковой флаг не задан).
	if forcedVersion != "" {
		info.LanguageVersion = forcedVersion
	}

	// Языковой флаг не задан — язык и framework оставляем детектированными.
	if lang == "" {
		return nil
	}

	// Перезаписываем информацию о проекте нужным языком.
	info.Language = lang
	// При форсировании языка версия от детектора к нему не относится —
	// сбрасываем, чтобы рантайм выбирался системный/по умолчанию,
	// если версия не задана явно через --version.
	if forcedVersion == "" {
		info.LanguageVersion = ""
	}
	switch lang {
	case "go":
		info.Framework = "go"
	case "javascript":
		info.Framework = "node"
	case "python":
		info.Framework = "python"
	case "php":
		info.Framework = "generic"
	}
	return nil
}

// flagName возвращает имя CLI-флага для внутреннего названия языка.
func flagName(lang string) string {
	if lang == "javascript" {
		return "js"
	}
	return lang
}

// detectProject определяет проект и сразу применяет принудительный язык
// из флагов --go/--php/--js/--python, если они заданы.
func detectProject(cwd string) (*detector.ProjectInfo, error) {
	info, err := detector.DetectProject(cwd)
	if err != nil {
		return nil, err
	}
	if err := applyForcedLanguage(info); err != nil {
		return nil, err
	}
	return info, nil
}

var dcrCmd = &cobra.Command{
	Use:   "dcr",
	Short: "Run docker-compose up -d and report",
	Run: func(cmd *cobra.Command, args []string) {
		runDcr()
	},
}

var prepareCmd = &cobra.Command{
	Use:   "prepare",
	Short: "Prepare project (permissions, env, vendors)",
	Run: func(cmd *cobra.Command, args []string) {
		runPrepare()
	},
}

var installCmd = &cobra.Command{
	Use:   "install [file]",
	Short: "Install dev (or specified file) to system",
	Long: `Install copies the dev executable (or a specified file) to a system directory.
If no file argument is provided, installs the currently running dev binary.
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

var virusCmd = &cobra.Command{
	Use:   "virus [user:pass@ip_addr]",
	Short: "Copy itself to remote server",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runVirus(args[0])
	},
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the project",
	Run: func(cmd *cobra.Command, args []string) {
		runBuild()
	},
}

// debugPort — порт для PHP-сервера в рамках dev debug.
var debugPort int

var debugCmd = &cobra.Command{
	Use:     "debug [params]",
	Aliases: []string{"dbg"},
	Short:   "Run the project under a debugger",
	Long: `Run the current project under a debugger.

For Go projects, Delve (dlv) is installed on demand and the program is run
under it: dlv debug <package> -- <params>. Any arguments after 'debug' are
passed to the debugged program.

For PHP projects, the local PHP must already have the Xdebug extension loaded;
the dev server is then started with Xdebug enabled, according to the project
framework (Symfony, Laravel, Yii or the built-in PHP server).

Examples:
  dev debug
  dev debug serve --addr=:8080
  dev debug --go
  dev debug --php`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runDebug(args)
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate()
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status with lock analysis",
	Long: `Analyze the current migration process status.

Shows detailed information about:
- Migration process (PHP PID, CPU, memory, state)
- Database connection (active queries, transactions, wait events)
- Lock chains (who blocks whom)
- Doctrine migration versions (executed, pending)
- Diagnosis and recommended action

Supports PostgreSQL and MySQL databases.`,
	Run: func(cmd *cobra.Command, args []string) {
		runMigrateStatus()
	},
}

var migrateNewCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Create a new empty migration",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) > 0 {
			name = args[0]
		}
		runMigrateNew(name)
	},
}

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Interactive database explorer",
	Long:  "Analyze databases in the project, connect, list tables, and view data.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := db.Run(); err != nil {
			color.Red("Error: %v", err)
		}
	},
}

func runAnalyze() {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("Dev version: %s\n", cyan(version.Version))
	fmt.Println()

	color.Cyan("=== Project Analysis ===")

	// Версия языка проекта (например "go 1.23"), если она определена.
	languageLabel := info.Language
	if info.LanguageVersion != "" {
		languageLabel = info.Language + " " + info.LanguageVersion
	}
	fmt.Printf("Language:  %s\n", green(languageLabel))
	fmt.Printf("Framework: %s\n", green(info.Framework))

	if info.HasEnv {
		fmt.Printf(".env:      %s\n", green("present"))
	} else {
		fmt.Printf(".env:      %s\n", red("missing"))
	}

	if info.HasVendor {
		fmt.Printf("Vendor:    %s\n", green("installed"))
	} else {
		fmt.Printf("Vendor:    %s\n", yellow("not installed"))
	}

	if len(info.DockerServices) > 0 {
		statuses, err := docker.GetServiceStatuses()
		if err != nil {
			// Если ошибка, выводим просто список
			fmt.Printf("Docker services: %s\n", cyan(strings.Join(info.DockerServices, ", ")))
		} else {
			var colored []string
			// Вспомогательная функция для короткого статуса
			shortStatus := func(status string) string {
				lower := strings.ToLower(status)
				switch {
				case strings.Contains(lower, "up"):
					return "up"
				case strings.Contains(lower, "exited") || strings.Contains(lower, "exit"):
					return "err"
				case strings.Contains(lower, "created"):
					return "up"
				case strings.Contains(lower, "paused"):
					return "warn"
				case strings.Contains(lower, "starting") || strings.Contains(lower, "restarting"):
					return "warn"
				default:
					return ""
				}
			}
			for _, svc := range info.DockerServices {
				status, ok := statuses[svc]
				if ok && status != "" {
					short := shortStatus(status)
					var coloredShort string
					switch short {
					case "up":
						coloredShort = green(short)
					case "err":
						coloredShort = red(short)
					case "warn":
						coloredShort = yellow(short)
					default:
						coloredShort = cyan(short)
					}
					colored = append(colored, svc+" ["+coloredShort+"]")
				} else {
					// Сервис без контейнера
					colored = append(colored, svc)
				}
			}
			if len(colored) == 0 {
				// Если все сервисы без контейнеров, выводим "none"
				fmt.Printf("Docker services: %s\n", yellow("none"))
			} else {
				fmt.Printf("Docker services: %s\n", strings.Join(colored, ", "))
			}
		}
	} else {
		fmt.Printf("Docker services: %s\n", yellow("none"))
	}

	if len(info.MakeCommands) > 0 {
		fmt.Printf("Make commands:   %s\n", cyan(strings.Join(info.MakeCommands, ", ")))
	} else {
		fmt.Printf("Make commands:   %s\n", yellow("none"))
	}

	if len(info.DevCommands) > 0 {
		fmt.Printf("Dev commands:    %s\n", cyan(strings.Join(info.DevCommands, ", ")))
	}

	// Databases
	if len(info.Databases) > 0 {
		fmt.Printf("Databases:       ")
		var dbStrs []string
		for _, db := range info.Databases {
			loc := ""
			switch db.Location {
			case detector.LocationLocal:
				loc = "local"
			case detector.LocationDocker:
				loc = "docker"
			case detector.LocationRemote:
				loc = "remote"
			default:
				loc = db.Location
			}
			dbName := db.Database
			if dbName == "" {
				dbName = db.Type // если имя БД не указано, используем тип
			}
			dbStrs = append(dbStrs, fmt.Sprintf("%s [%s] (%s:%s, %s)", dbName, loc, db.Host, db.Port, db.Type))
		}
		fmt.Printf("%s\n", cyan(strings.Join(dbStrs, ", ")))
	} else {
		fmt.Printf("Databases:       %s\n", yellow("none detected"))
	}

	fmt.Println()
}

func runCache() {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Yellow("Clearing cache for %s (%s)...", info.Framework, info.Language)
	err = cache.ClearCache(info.Framework)
	if err != nil {
		color.Red("Failed to clear cache: %v", err)
		return
	}
	color.Green("Cache cleared successfully.")
}

func runLogs() {
	cwd, _ := os.Getwd()
	entries, err := logs.FindLogs(cwd)
	if err != nil {
		color.Red("Error finding logs: %v", err)
		return
	}

	if len(entries) == 0 {
		color.Yellow("No log files or docker containers found.")
		return
	}

	color.Cyan("Available logs:")
	for i, entry := range entries {
		typ := entry.Type
		if typ == "docker" {
			color.Yellow("  %d) [docker] %s", i+1, entry.Path)
		} else {
			color.White("  %d) [file]   %s", i+1, entry.Path)
		}
	}

	fmt.Print("\nSelect log number to open in lnav (or 0 to exit): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" || input == "0" {
		return
	}
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(entries) {
		color.Red("Invalid selection")
		return
	}

	selected := entries[idx-1]
	color.Green("Opening %s (%s)...", selected.Path, selected.Type)
	err = logs.OpenLogInLnav(selected.Path, selected.Type)
	if err != nil {
		color.Red("Failed to open log: %v", err)
	}
}

func runRun() {
	cwd, _ := os.Getwd()
	info, err := detectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Running project: %s (%s)", info.Framework, info.Language)
	opts := run.RunOptions{Port: runPort, PublicDir: info.PublicDir, Version: info.LanguageVersion}
	err = run.RunProjectWithOptions(info.Framework, info.Language, opts)
	if err != nil {
		color.Red("Failed to run project: %v", err)
	}
}

func runDcr() {
	color.Cyan("Starting docker-compose...")
	err := docker.ComposeUp()
	if err != nil {
		color.Red("Docker compose failed: %v", err)
	}
}

func runPrepare() {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Yellow("Preparing project...")
	err = prepare.PrepareProject(info.Framework, info.Language)
	if err != nil {
		color.Red("Preparation failed: %v", err)
		return
	}
	color.Green("Project prepared successfully.")
}

func runInstall(file string) {
	color.Cyan("Installing dev...")
	err := install.Install(file)
	if err != nil {
		color.Red("Install failed: %v", err)
		return
	}
	color.Green("Installation successful.")
}

func runVirus(path string) {
	color.Cyan("Copying to remote server %s...", path)
	err := virus.Virus(path)
	if err != nil {
		color.Red("Virus command failed: %v", err)
		return
	}
	color.Green("Copy successful.")
}

func runBuild() {
	cwd, _ := os.Getwd()
	info, err := detectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Building project: %s (%s)", info.Framework, info.Language)
	err = build.BuildProject(info.Framework, info.Language, info.LanguageVersion)
	if err != nil {
		color.Red("Build failed: %v", err)
		return
	}
	color.Green("Build successful.")
}

func runDebug(params []string) {
	cwd, _ := os.Getwd()
	info, err := detectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Debugging project: %s (%s)", info.Framework, info.Language)
	opts := debug.Options{
		Framework: info.Framework,
		Language:  info.Language,
		Version:   info.LanguageVersion,
		PublicDir: info.PublicDir,
		Params:    params,
		Port:      debugPort,
	}
	if err := debug.Run(opts); err != nil {
		color.Red("Debug failed: %v", err)
	}
}

func runMigrate() {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Running migrations for %s (%s)", info.Framework, info.Language)
	err = migrate.RunMigrations(info.Framework, info.Language)
	if err != nil {
		color.Red("Migration failed: %v", err)
		return
	}
	color.Green("Migrations completed successfully.")
}

func runMigrateStatus() {
	err := migrate.RunMigrationStatus()
	if err != nil {
		color.Red("Migration status error: %v", err)
	}
}

func runMigrateNew(name string) {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Creating new migration for %s (%s)", info.Framework, info.Language)
	err = migrate.CreateNewMigration(info.Framework, info.Language, name)
	if err != nil {
		color.Red("Failed to create migration: %v", err)
		return
	}
	color.Green("Migration created successfully.")
}

var portCmd = &cobra.Command{
	Use:   "port [address]",
	Short: "Check if port is occupied and show process info",
	Long: `Check if a port is occupied and show detailed information about the process using it.
Uses lsof, ss, and nmap for detection.

Examples:
  dev port 127.0.0.1:1000
  dev port :8080
  dev port localhost:3306`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runPortCheck(args[0])
	},
}

var curlCmd = &cobra.Command{
	Use:   "curl <url> [method]",
	Short: "Make HTTP request and show/save response",
	Long: `Make an HTTP request to the specified URL and interactively choose
to display the response or save it to a file.

Automatically prepends https:// if no protocol is specified.
Uses --insecure mode (skips TLS certificate verification).

Methods: GET (default), POST, PUT, DELETE

Examples:
  dev curl example.com
  dev curl example.com POST
  dev curl https://api.example.com PUT
  dev curl http://localhost:8080/api DELETE`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		url := args[0]
		method := "GET"
		if len(args) > 1 {
			method = args[1]
		}
		runCurl(url, method)
	},
}

func runPortCheck(addr string) {
	err := port.CheckPort(addr)
	if err != nil {
		color.Red("Error: %v", err)
	}
}

func runCurl(url, method string) {
	err := curl.Run(url, method)
	if err != nil {
		color.Red("Error: %v", err)
	}
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update dev to the latest version",
	Long: `Download the latest dev binary from GitHub releases and install it.
The binary is downloaded to the home directory, installed via 'dev install',
and then the temporary file is removed.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := update.SelfUpdate(); err != nil {
			color.Red("Error: %v", err)
		}
	},
}

var selfConfigCmd = &cobra.Command{
	Use:   "self-config",
	Short: "Open AI configuration file for editing",
	Long: `Open the AI configuration file (~/dev-config/main.conf) for editing.
Creates the file with default empty parameters if it doesn't exist.
Uses $EDITOR or nano by default.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := ai.EditConfig(); err != nil {
			color.Red("Error: %v", err)
		}
	},
}

var selfCommandCmd = &cobra.Command{
	Use:   "self-command",
	Short: "Open custom commands config for editing",
	Long: `Open the custom commands config (~/dev-command/custom.yml) for editing.
Creates the file with a default template if it doesn't exist.
Uses $EDITOR or nano by default.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := custom.Edit(); err != nil {
			color.Red("Error: %v", err)
		}
	},
}

var aiCmd = &cobra.Command{
	Use:   "ai <text>",
	Short: "Ask AI to generate and execute commands",
	Long: `Send a request to an AI model (OpenAI-compatible API) to generate
a list of terminal commands based on your description.

The AI analyzes the current project context and suggests commands.
You can then choose to execute them one by one or refine the request.

Configuration is stored in ~/dev-config/main.conf (user config)
or /etc/dev-command/main.conf (system fallback).
Use 'dev self-config' to set up your API endpoint, token, and model.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		text := strings.Join(args, " ")
		if err := ai.RunAI(text); err != nil {
			color.Red("Error: %v", err)
		}
	},
}

var checkAll bool
var checkCommit int
var checkBranch string
var checkCode bool

var checkCmd = &cobra.Command{
	Use:     "review",
	Aliases: []string{"check"},
	Short:   "Run static code analysis with linters",
	Long: `Run static code analysis on the current project using linters
appropriate for the detected language and framework.

By default the analysis runs in dry-run mode (issues are only reported).
Use 'dev review fix' to automatically fix fixable issues.

Available non-interactive flags to choose the scope:
	 dev review --all
	 dev review --commit=3
	 dev review --branch=master
	 dev review --code`,
	Run: func(cmd *cobra.Command, args []string) {
		runCheck(check.Options{})
	},
}

var checkFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Run static analysis and fix issues",
	Long: `Run static code analysis and automatically fix issues
where the tooling supports it.`,
	Run: func(cmd *cobra.Command, args []string) {
		runCheck(check.Options{Mode: check.ModeFix})
	},
}

var checkAICmd = &cobra.Command{
	Use:   "ai [text]",
	Short: "AI-powered code review",
	Long: `Send the changed code to a neural network for an AI code review.

The command collects the changed files (git diff), asks what to send,
and requests the AI to find real, critical problems without inventing
issues that do not exist. The AI response is then printed.

You can pass an optional instruction, for example:
	 dev review ai Проверь на уязвимости нулевого дня

Use the same scope flags as 'dev review':
	 dev review ai --all
	 dev review ai --commit=3
	 dev review ai --branch=master`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runCheckAI(strings.Join(args, " "))
	},
}

func init() {
	// PersistentFlags — подкоманды fix/ai наследуют эти флаги.
	checkCmd.PersistentFlags().BoolVar(&checkAll, "all", false, "Check all code")
	checkCmd.PersistentFlags().IntVar(&checkCommit, "commit", 0, "Check changed code plus last N commits")
	checkCmd.PersistentFlags().StringVar(&checkBranch, "branch", "", "Check diff with the given branch (master|develop)")
	checkCmd.PersistentFlags().BoolVar(&checkCode, "code", false, "Check only changed code")
	addLanguageFlags(checkCmd, true)
}

// runCheck запускает статическую проверку кода.
// Неинтерактивные флаги имеют приоритет; при их отсутствии — интерактивный выбор.
func runCheck(opts check.Options) {
	cwd, _ := os.Getwd()
	applyCheckScopeFlags(&opts)

	info, err := detectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	if err := check.Run(info, opts); err != nil {
		color.Red("Check failed: %v", err)
	}
}

// runCheckAI запускает AI-код-ревью изменённого кода.
// Использует те же флаги выбора объёма, что и обычный dev check.
// text — необязательная инструкция пользователя к ревью.
func runCheckAI(text string) {
	cwd, _ := os.Getwd()
	opts := check.Options{}
	applyCheckScopeFlags(&opts)

	info, err := detectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	if err := check.RunAI(info, opts, text); err != nil {
		color.Red("AI check failed: %v", err)
	}
}

// applyCheckScopeFlags заполняет Options в зависимости от флагов:
// --all, --commit=N, --branch=NAME. Если ни один не задан — интерактивный выбор.
func applyCheckScopeFlags(opts *check.Options) {
	switch {
	case checkAll:
		s := check.ScopeAll()
		opts.Scope = &s
	case checkCommit > 0:
		s := check.ScopeCommits(checkCommit)
		opts.Scope = &s
	case checkBranch != "":
		s := check.ScopeDiff(checkBranch)
		opts.Scope = &s
	case checkCode:
		s := check.ScopeChanged()
		opts.Scope = &s
	default:
		opts.Interactive = true
	}
}

// runRoot обрабатывает вызовы корневой команды.
// Без аргументов выполняется анализ проекта. Незнакомая команда сверяется
// с пользовательскими командами из ~/dev-command/custom.yml и, если найдена,
// запускается с пробросом параметров: текущий путь, язык и фреймворк.
func runRoot(args []string) {
	if len(args) == 0 {
		runAnalyze()
		return
	}

	name := args[0]
	cwd, _ := os.Getwd()

	// Определяем проект, чтобы пробросить язык и фреймворк в команду.
	// Ошибка детекции не блокирует запуск пользовательской команды.
	lang, framework := "", ""
	if info, err := detectProject(cwd); err == nil {
		lang = info.Language
		framework = info.Framework
	}

	cfg, err := custom.Load()
	if err != nil {
		color.Red("Failed to load custom commands: %v", err)
		os.Exit(1)
		return
	}

	ctx := custom.Context{Dir: cwd, Language: lang, Framework: framework}
	found, err := cfg.RunCommand(name, ctx)
	if err != nil {
		color.Red("Custom command %q failed: %v", name, err)
		os.Exit(1)
		return
	}
	if !found {
		color.Red("Unknown command %q for \"dev\"", name)
		if names := cfg.Names(); len(names) > 0 {
			color.Yellow("Available custom commands: %s", strings.Join(names, ", "))
		}
		fmt.Fprintln(os.Stderr, "Run 'dev --help' for usage.")
		os.Exit(1)
	}
}

func main() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(cacheCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(dcrCmd)
	rootCmd.AddCommand(prepareCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(virusCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(portCmd)
	rootCmd.AddCommand(curlCmd)
	rootCmd.AddCommand(selfUpdateCmd)
	rootCmd.AddCommand(selfConfigCmd)
	rootCmd.AddCommand(selfCommandCmd)
	rootCmd.AddCommand(aiCmd)
	rootCmd.AddCommand(checkCmd)
	checkCmd.AddCommand(checkFixCmd)
	checkCmd.AddCommand(checkAICmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateNewCmd)
	// Разрешаем произвольные аргументы на корне, чтобы перехватывать
	// незнакомые команды и сверять их с пользовательскими командами.
	rootCmd.Args = cobra.ArbitraryArgs
	// Default action is analyze; незнакомые команды обрабатываются в runRoot.
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		runRoot(args)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
