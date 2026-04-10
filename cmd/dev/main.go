package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"dev/internal/build"
	"dev/internal/cache"
	"dev/internal/detector"
	"dev/internal/docker"
	"dev/internal/install"
	"dev/internal/logs"
	"dev/internal/migrate"
	"dev/internal/prepare"
	"dev/internal/run"
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
	Use:   "logs",
	Short: "Show logs and open in lnav",
	Run: func(cmd *cobra.Command, args []string) {
		runLogs()
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the project",
	Run: func(cmd *cobra.Command, args []string) {
		runRun()
	},
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

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate()
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
	fmt.Printf("Language:  %s\n", green(info.Language))
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
		fmt.Printf("Docker services: %s\n", cyan(strings.Join(info.DockerServices, ", ")))
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
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Running project: %s (%s)", info.Framework, info.Language)
	err = run.RunProject(info.Framework, info.Language)
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
	info, err := detector.DetectProject(cwd)
	if err != nil {
		color.Red("Error detecting project: %v", err)
		return
	}

	color.Green("Building project: %s (%s)", info.Framework, info.Language)
	err = build.BuildProject(info.Framework, info.Language)
	if err != nil {
		color.Red("Build failed: %v", err)
		return
	}
	color.Green("Build successful.")
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
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateNewCmd)
	// Default action is analyze
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		runAnalyze()
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
