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
  - Go: detects main files, offers selection, builds executable (`-o/--output` sets the output file name)
  - Node.js: runs `npm run build`
  - Other languages: no‑op (informs that building is not required)

- **`dev unit [args]`** – Run unit tests with the appropriate runner for the detected language/framework:
  - Go: `go test ./...`
  - PHP: `vendor/bin/phpunit` (falls back to the `composer test` script)
  - JS/Node: `npm test` / `yarn test` / `pnpm test` (detected from lock files)
  - Python: `pytest` (if configured or installed), otherwise `manage.py test` (Django) or `unittest discover`
  - Ruby: `bin/rails test` / `bundle exec rspec` / `bundle exec rake test`
  - Extra arguments are passed through to the test runner, e.g. `dev unit ./internal/...`
  - Supports the same language flags as other commands (`--go`, `--php`, `--js`, `--python`, `--version`)

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
  - Non-interactive flags: `--all`, `--commit=N`, `--branch=master|develop`, `--code`

- **`prod release`** – Production release management (prepare + switch):
  - `prod release prepare [name]` – move build artifacts from `builds_folder` into a new `releases_folder/release-<datetime>` archive folder; interactive release name selection (default: first) when omitted
  - `prod release switch [name]` – list the most recent releases (newest first, today's releases highlighted with a white background) and switch the `current_release_folder` symlink to the selected one; `-l/--lines` controls how many releases are shown (default: 5)
  - `prod release` – run both steps in order: prepare a new release, then switch to it
  - Configuration is read from `release.yml` in the current directory; if missing or invalid, an editor opens with a filled template (same behavior as `dev self-config`)
  - Per-release optional settings: `release_prefix` (default `release-`) and `release_time_format` (default `2006-01-02_15-04-05`)

- **`prod virus [user@ip_addr]`** – Copy the prod executable to a remote server via SCP and install the production configuration there:
  - `/etc/prod-command` config files (e.g. `deps.conf`; the `reports` history is not copied) — installed via `sudo` for non-root users
  - LLM config (`~/dev-config/main.conf` or `/etc/dev-command/main.conf`) so `prod llm` works right away
  - Supports `user@host` or just `ip` formats (SSH key auth only)

- **`prod install [file]`** – Install the prod tool (or a specified executable) to a system directory (`/usr/local/bin`, `~/bin`, etc.) with interactive directory selection.

- **`prod self-update`** – Download and install the latest version of prod from GitHub releases.

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
dev unit                # run unit tests
dev unit ./internal/... # run unit tests for specific packages
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

prod                    # production server health report
prod release            # prepare a new release and switch to it
prod release prepare    # move build artifacts to releases/release-<datetime>
prod release switch -l 5  # switch the current release symlink
prod virus user@host    # copy prod to remote server
prod install            # install prod to system
prod self-update        # update prod to latest version
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

### Custom Commands

Unknown `dev <name>` commands are matched against custom commands defined in:

- `~/dev-command/custom.yml` — global commands (edit with `dev self-command`)
- `.custom` in the directory where `dev` is invoked — project-local commands; local commands override global ones with the same name

Command format:

```yaml
commands:
  deploy:
    subcommands:
      - git pull
      - dev migrate
    # path restricts the command: it runs only when the current directory
    # is inside the given path (absolute, "~", or relative to the launch dir).
    # Empty value means the command is available everywhere.
    path: /home/user/myproject
```

Available variables in subcommands: `$(current_dir)`, `$(language)`, `$(framework)`.

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
│   ├── prod/                # Production server health diagnostics
│   ├── release/             # Release management (release.yml, symlinks)
│   ├── run/                 # Project runner
│   ├── unit/                # Unit test running
│   ├── update/              # Self-update logic (dev/prod)
│   ├── version/             # Version information
│   └── virus/               # Remote copy via SCP
├── cmd/prod/main.go         # prod CLI entry point (health + release)
├── go.mod
└── README.md
```

## License

CC-BY-NC-4.0
