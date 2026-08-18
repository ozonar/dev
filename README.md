# Dev CLI Tool

A command-line tool to assist with development tasks: analyze projects, clear caches, view logs, run projects, manage Docker, prepare environments, work with databases, run migrations, check ports, make HTTP requests, and interact with AI.

## Features

- **`dev`** or **`dev analyze`** – Analyze current directory:
  - Detect language/framework (PHP, Go, Node.js, Python, etc.)
  - Check for `.env`, vendor installation, Docker services, Make commands
  - Detect databases (local, Docker, remote)
  - Colorful output with status indicators

- **`dev cache`** – Clear framework‑specific caches:
  - Symfony: `bin/console cache:clear`
  - Laravel: `php artisan cache:clear`
  - Yii: `php yii cache/flush-all`
  - Go: `go clean -cache -modcache -testcache`
  - Node.js: `npm cache clean --force`
  - Python: remove `__pycache__` and `*.pyc`
  - Generic PHP: cleans `cache` folders

- **`dev logs`** – Find log files and Docker container logs, then open them in `lnav` (interactive selection)

- **`dev run [port]`** – Start the project with the appropriate runner:
  - Symfony: `symfony serve`
  - Laravel: `php artisan serve`
  - Go: `go run` (auto‑detects main files)
  - Node.js: `npm run dev`
  - Python: `python manage.py runserver` or simple HTTP server
  - Supports `--port` flag or positional port argument (default: 8000)

- **`dev dcr`** – Run `docker-compose up -d` and report running services

- **`dev prepare`** – Prepare the project for development:
  - Set `777` permissions on cache directories
  - Copy `.env.dist` / `.env.dev` to `.env`
  - Install/reinstall vendors (composer, npm, go mod, pip)

- **`dev install [file]`** – Install the dev tool (or a specified executable) to a system directory (`/usr/local/bin`, `~/bin`, etc.) with interactive directory selection.

- **`dev self-update`** – Download and install the latest version of dev from GitHub releases.

- **`dev virus [user:pass@ip_addr]`** – Copy the dev executable to a remote server via SCP (supports `user@host` or `user:pass@ip` formats). Automatically sets execute permissions.

- **`dev build`** – Build the project according to its language:
  - Go: detects main files, offers selection, builds executable
  - Node.js: runs `npm run build`
  - Other languages: no‑op (informs that building is not required)

- **`dev migrate`** – Run database migrations for the detected framework/language.

- **`dev migrate status`** – Show migration status with lock analysis:
  - Migration process (PHP PID, CPU, memory, state)
  - Database connection (active queries, transactions, wait events)
  - Lock chains (who blocks whom)
  - Doctrine migration versions (executed, pending)
  - Diagnosis and recommended action
  - Supports PostgreSQL and MySQL databases.

- **`dev migrate new [name]`** – Create a new empty migration file.

- **`dev db`** – Interactive database explorer: analyze databases in the project, connect, list tables, and view data.

- **`dev port <address>`** – Check if a port is occupied and show detailed process information:
  - Uses `fuser`, `ss`, `lsof` for local port detection
  - Offers `nmap` scan for service detection
  - Supports remote hosts (auto‑runs nmap)
  - Formats: `127.0.0.1:1000`, `:8080`, `8080`

- **`dev curl <url> [method]`** – Make an HTTP request and interactively choose to display the response or save it to a file:
  - Automatically prepends `https://` if no protocol is specified
  - Uses `--insecure` mode (skips TLS certificate verification)
  - Methods: `GET` (default), `POST`, `PUT`, `DELETE`
  - Shows status, duration, and content length

- **`dev ai <text>`** – Send a request to an AI model (OpenAI-compatible API) to generate and execute terminal commands:
  - AI analyzes the current project context and suggests commands
  - Interactive loop: execute commands one by one or refine the request
  - Configuration via `~/dev-config/main.conf` or `/etc/dev-command/main.conf`

- **`dev self-config`** – Open AI configuration file for editing:
  - Creates the file with default empty parameters if it doesn't exist
  - Uses `$EDITOR` or `nano` by default
  - Required parameters: `LLM_ENDPOINT`, `LLM_TOKEN`, `LLM_MODEL`

- **`dev check`** – Run static code analysis with linters for the detected language/framework:
  - Go: `golangci-lint`
  - PHP: `phpstan` (with `--level=5`, `--memory-limit=1G`) and `php-cs-fixer`
  - Downloads required tools (and php runtime) to `~/dev-config/check` if not present
  - Interactive selection of the check scope (changed code, changed code + N commits, all code, diff with master/develop)
  - Runs by default in **dry-run** mode; output is streamed to the console
  - Subcommands: `dev check fix` (auto-fix issues), `dev check ai` (AI review, planned)
  - Non-interactive flags: `--all`, `--commit=N`, `--branch=master|develop`

## Installation

### From GitHub

```bash
wget -O dev https://github.com/ozonar/dev/releases/latest/download/dev-linux-amd64 && chmod +x dev
```

```bash
./dev install
```

## Usage

Navigate to your project directory and run:

```bash
dev                     # analyze project
dev cache               # clear cache
dev logs                # show logs
dev run                 # run project
dev run 8080            # run project on port 8080
dev dcr                 # start docker-compose
dev prepare             # prepare environment
dev install             # install dev to system
dev self-update         # update dev to latest version
dev virus user@host     # copy dev to remote server
dev build               # build project
dev migrate             # run database migrations
dev migrate status      # show migration status
dev migrate new         # create a new migration
dev db                  # interactive database explorer
dev port :8080          # check if port is occupied
dev curl example.com    # make HTTP request
dev ai "install npm"    # ask AI to generate commands
dev self-config         # configure AI settings
dev check               # run static code analysis (dry-run)
dev check fix           # run analysis and auto-fix issues
```

## Configuration

### AI Configuration

The AI feature requires configuration in `~/dev-config/main.conf`:

```ini
LLM_ENDPOINT=https://api.openai.com/v1/chat/completions
LLM_TOKEN=your-api-token
LLM_MODEL=gpt-4o
```

Use `dev self-config` to open the config file for editing.

### Project Detection

No configuration files are required for project detection. The tool automatically detects your project based on common file patterns.

## Optional Requirements

- Docker & docker-compose (optional, for `dev dcr`)
- lnav (optional, for `dev logs` interactive viewing)
- nmap (optional, for `dev port` service detection)
- Framework-specific tools (optional, php, npm, go, python, etc.)
- SSH keys (optional, for `dev virus`)

## Project Structure

```
dev/
├── cmd/dev/main.go          # CLI entry point
├── internal/
│   ├── ai/                  # AI integration (OpenAI-compatible API)
│   ├── ai/config.go         # AI configuration management
│   ├── build/               # Project building
│   ├── cache/               # Cache clearing
│   ├── check/               # Static code analysis
│   ├── colors/              # ANSI color helpers
│   ├── common/              # Shared utilities (file ops, commands)
│   ├── curl/                # HTTP request client
│   ├── db/                  # Database explorer
│   ├── detector/            # Project detection
│   ├── docker/              # Docker-compose operations
│   ├── install/             # Installation logic
│   ├── logs/                # Log discovery
│   ├── migrate/             # Database migrations
│   ├── migrate/status.go    # Migration status & lock analysis
│   ├── port/                # Port checking (fuser, ss, lsof, nmap)
│   ├── prepare/             # Environment preparation
│   ├── run/                 # Project runner
│   ├── version/             # Version information
│   └── virus/               # Remote copy via SCP
├── go.mod
└── README.md
```

## License

CC-BY-NC-4.0
