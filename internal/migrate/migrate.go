package migrate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
)

// RunMigrations запускает миграции в зависимости от фреймворка
func RunMigrations(framework, language string) error {
	switch framework {
	case "laravel":
		return runLaravelMigrations()
	case "rails":
		return runRailsMigrations()
	case "django":
		return runDjangoMigrations()
	case "go":
		return runGoMigrateMigrations()
	case "node":
		return runNodeMigrations()
	case "symfony":
		return runSymfonyMigrations()
	case "yii":
		return runYiiMigrations()
	default:
		return fmt.Errorf("migrations for framework %s are not supported", framework)
	}
}

// CreateNewMigration создает новую пустую миграцию
func CreateNewMigration(framework, language, name string) error {
	if name == "" {
		name = "new_migration"
	}

	switch framework {
	case "laravel":
		return createLaravelMigration(name)
	case "rails":
		return createRailsMigration(name)
	case "django":
		return createDjangoMigration(name)
	case "go":
		return createGoMigrateMigration(name)
	case "node":
		return createNodeMigration(name)
	case "symfony":
		return createSymfonyMigration(name)
	case "yii":
		return createYiiMigration(name)
	default:
		return fmt.Errorf("creating migrations for framework %s is not supported", framework)
	}
}

// runLaravelMigrations запускает миграции Laravel
func runLaravelMigrations() error {
	color.Cyan("Running Laravel migrations...")
	cmd := exec.Command("php", "artisan", "migrate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runRailsMigrations запускает миграции Rails
func runRailsMigrations() error {
	color.Cyan("Running Rails migrations...")
	cmd := exec.Command("rails", "db:migrate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDjangoMigrations запускает миграции Django
func runDjangoMigrations() error {
	color.Cyan("Running Django migrations...")
	cmd := exec.Command("python", "manage.py", "migrate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runSymfonyMigrations запускает миграции Symfony
func runSymfonyMigrations() error {
	color.Cyan("Running Symfony migrations...")
	cmd := exec.Command("php", "bin/console", "doctrine:migrations:migrate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runYiiMigrations запускает миграции Yii
func runYiiMigrations() error {
	color.Cyan("Running Yii migrations...")
	cmd := exec.Command("php", "yii", "migrate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGoMigrateMigrations запускает миграции с использованием go-migrate
func runGoMigrateMigrations() error {
	color.Cyan("Running go migrations...")
	// Проверяем наличие утилиты migrate
	if _, err := exec.LookPath("migrate"); err != nil {
		return fmt.Errorf("migrate utility not found. Install: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest")
	}

	// Ищем файлы миграций в стандартных директориях
	possibleDirs := []string{"migrations", "db/migrations", "internal/migrations"}
	var migrationDir string
	for _, dir := range possibleDirs {
		if _, err := os.Stat(dir); err == nil {
			migrationDir = dir
			break
		}
	}

	if migrationDir == "" {
		return fmt.Errorf("migration directory not found. Check for migrations, db/migrations or internal/migrations folder")
	}

	// Получаем DSN из переменных окружения
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/db?sslmode=disable"
		color.Yellow("Using default DSN: %s. Set DATABASE_URL environment variable to change.", dsn)
	}

	cmd := exec.Command("migrate", "-path", migrationDir, "-database", dsn, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runNodeMigrations запускает миграции для Node.js проектов
func runNodeMigrations() error {
	color.Cyan("Running Node.js migrations...")

	// Пытаемся определить скрипт миграций
	scripts := []string{"migrate", "db:migrate", "knex:migrate", "typeorm:migration:run"}
	for _, script := range scripts {
		cmd := exec.Command("npm", "run", script)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Если не нашли скрипт, пытаемся использовать knex или typeorm напрямую
	if _, err := exec.LookPath("knex"); err == nil {
		cmd := exec.Command("knex", "migrate:latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if _, err := exec.LookPath("typeorm"); err == nil {
		cmd := exec.Command("typeorm", "migration:run")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("could not find migration tool (knex, typeorm) or script in package.json")
}

// createLaravelMigration создает новую миграцию Laravel
func createLaravelMigration(name string) error {
	color.Cyan("Creating Laravel migration: %s", name)
	cmd := exec.Command("php", "artisan", "make:migration", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createRailsMigration создает новую миграцию Rails
func createRailsMigration(name string) error {
	color.Cyan("Creating Rails migration: %s", name)
	cmd := exec.Command("rails", "generate", "migration", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createDjangoMigration создает новую миграцию Django
func createDjangoMigration(name string) error {
	color.Cyan("Creating Django migration: %s", name)
	cmd := exec.Command("python", "manage.py", "makemigrations", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createSymfonyMigration создает новую миграцию Symfony
func createSymfonyMigration(name string) error {
	color.Cyan("Creating Symfony migration")
	// Symfony использует doctrine:migrations:generate для создания миграции
	cmd := exec.Command("php", "bin/console", "doctrine:migrations:generate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createYiiMigration создает новую миграцию Yii
func createYiiMigration(name string) error {
	color.Cyan("Creating Yii migration: %s", name)
	cmd := exec.Command("php", "yii", "migrate/create", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createGoMigrateMigration создает новую миграцию go-migrate
func createGoMigrateMigration(name string) error {
	color.Cyan("Creating go-migrate migration: %s", name)

	// Генерируем имя файла с timestamp
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.up.sql", timestamp, strings.ToLower(strings.ReplaceAll(name, " ", "_")))

	// Определяем директорию для миграций
	possibleDirs := []string{"migrations", "db/migrations", "internal/migrations"}
	var migrationDir string
	for _, dir := range possibleDirs {
		if _, err := os.Stat(dir); err == nil {
			migrationDir = dir
			break
		}
	}

	if migrationDir == "" {
		// Создаем директорию migrations если её нет
		migrationDir = "migrations"
		if err := os.MkdirAll(migrationDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", migrationDir, err)
		}
	}

	fullPath := filepath.Join(migrationDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create migration file: %v", err)
	}
	defer file.Close()

	// Пишем шаблон SQL
	template := `-- Migration: %s
-- Created at: %s

-- Add your SQL here
-- Example:
-- CREATE TABLE IF NOT EXISTS %s (
--     id SERIAL PRIMARY KEY,
--     created_at TIMESTAMP DEFAULT NOW()
-- );

SELECT 'Migration %s applied successfully' as result;
`
	content := fmt.Sprintf(template, name, time.Now().Format(time.RFC3339),
		strings.ToLower(strings.ReplaceAll(name, " ", "_")), name)

	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write to migration file: %v", err)
	}

	// Создаем соответствующий down файл
	downFilename := fmt.Sprintf("%s_%s.down.sql", timestamp, strings.ToLower(strings.ReplaceAll(name, " ", "_")))
	downFilepath := filepath.Join(migrationDir, downFilename)
	downFile, err := os.Create(downFilepath)
	if err != nil {
		return fmt.Errorf("failed to create down migration file: %v", err)
	}
	defer downFile.Close()

	downTemplate := `-- Migration down: %s
-- Reverts the changes made in the up migration

-- Add your SQL here to revert the migration
-- Example:
-- DROP TABLE IF EXISTS %s;

SELECT 'Migration %s reverted successfully' as result;
`
	downContent := fmt.Sprintf(downTemplate, name,
		strings.ToLower(strings.ReplaceAll(name, " ", "_")), name)

	if _, err := downFile.WriteString(downContent); err != nil {
		return fmt.Errorf("failed to write to down migration file: %v", err)
	}

	color.Green("Migration files created:")
	color.Green("  %s", fullPath)
	color.Green("  %s", downFilepath)

	return nil
}

// createNodeMigration создает новую миграцию для Node.js
func createNodeMigration(name string) error {
	color.Cyan("Creating Node.js migration: %s", name)

	// Пытаемся использовать knex
	if _, err := exec.LookPath("knex"); err == nil {
		cmd := exec.Command("knex", "migrate:make", name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Пытаемся использовать typeorm
	if _, err := exec.LookPath("typeorm"); err == nil {
		cmd := exec.Command("typeorm", "migration:create", name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Если не нашли инструменты, создаем простой файл миграции
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.js", timestamp, strings.ToLower(strings.ReplaceAll(name, " ", "_")))
	migrationDir := "migrations"

	if err := os.MkdirAll(migrationDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", migrationDir, err)
	}

	fullPath := filepath.Join(migrationDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create migration file: %v", err)
	}
	defer file.Close()

	template := `// Migration: %s
// Created at: %s

exports.up = function(knex) {
  // Add your migration logic here
  // Example:
  // return knex.schema.createTable('%s', function(table) {
  //   table.increments('id');
  //   table.timestamps(true, true);
  // });
};

exports.down = function(knex) {
  // Revert the changes made in up()
  // Example:
  // return knex.schema.dropTableIfExists('%s');
};
`
	content := fmt.Sprintf(template, name, time.Now().Format(time.RFC3339),
		strings.ToLower(strings.ReplaceAll(name, " ", "_")),
		strings.ToLower(strings.ReplaceAll(name, " ", "_")))

	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write to migration file: %v", err)
	}

	color.Green("Migration file created: %s", fullPath)
	return nil
}
