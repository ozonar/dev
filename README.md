# Dev CLI Tool

A command-line tool to assist with development tasks: run and build projects, static and llm-specific review code, interact with LLM, explore databases, view logs, run migrations, clear caches, run unit tests, prepare environments, manage Docker, use recipes, check ports and make HTTP requests.

## Features

- **`dev run [target]`** – Start the project with the appropriate runner:
  - Symfony: `symfony serve`
  - Laravel: `php artisan serve`
  - Go: `go run` (auto‑detects main)
  - Node.js: `npm run dev`
  - Python: `python manage.py runserver` or simple HTTP server
  - Supports `--port`/`-p` flag (default: 8000)

- **`dev build [target]`** – Build the project according to its language:
  - Go: builds the package as a whole. `target` may be a `.go` file, a package directory or an import path (e.g. `dev build cmd/prod/main.go`, `dev build ./cmd/prod`). Without `target` main packages are detected automatically and you are asked to choose one if several are found. `-o/--output` sets the output file name
  - Node.js: runs `npm run build`
  - Other languages: no‑op (informs that building is not required)

- **`dev review`** – Run static code analysis with linters for the detected language/framework:
  - Go: `golangci-lint`
  - PHP: `phpstan` (with `--level=5`, `--memory-limit=1G`) and `php-cs-fixer`
  - Downloads required tools (and php runtime) to `~/dev-command/check` if not present
  - Interactive selection of the check scope (changed code, changed code + N commits, all code, diff with master/develop)
  - Runs by default in **dry-run** mode; output is streamed to the console
  - Subcommands: `dev review fix` (auto-fix issues), `dev review ai` (AI code review)
  - Non-interactive flags: `--all`, `--commit=N`, `--branch=NAME`, `--code`

- **`dev ai <text>`** – Send a request to an AI model (OpenAI-compatible API) to generate and execute terminal commands:
  - AI analyzes the current project context and suggests commands
  - Interactive loop: execute commands one by one or refine the request
  - Configuration via `~/dev-command/main.conf` or `/etc/dev-command/main.conf`

- **`dev db`** – Interactive database explorer: analyze databases in the project, connect, list tables, and view data.

- **`dev logs`** – Find log files and Docker container logs, then open them in `lnav` (interactive selection)

- **`dev migrate`** – Run database migrations for the detected framework/language.

- **`dev migrate status`** – Show migration status with lock analysis:
  - Migration process (PHP PID, CPU, memory, state)
  - Database connection (active queries, transactions, wait events)
  - Lock chains (who blocks whom)
  - Doctrine migration versions (executed, pending)
  - Diagnosis and recommended action
  - Supports PostgreSQL and MySQL databases.

- **`dev migrate new [name]`** – Create a new empty migration file.

- **`dev cache`** – Clear framework‑specific caches:
  - Symfony: `bin/console cache:clear`
  - Laravel: `php artisan cache:clear`
  - Yii: `php yii cache/flush-all`
  - Go: `go clean -cache -modcache -testcache`
  - Node.js: `npm cache clean --force`
  - Python: remove `__pycache__` and `*.pyc`
  - Generic PHP: cleans `cache` folders

- **`dev unit [args]`** – Run unit tests with the appropriate runner for the detected language/framework:
  - Go: `go test ./...`
  - PHP: `vendor/bin/phpunit` (falls back to the `composer test` script)
  - JS/Node: `npm test` / `yarn test` / `pnpm test` (detected from lock files)
  - Python: `pytest` (if configured or installed), otherwise `manage.py test` (Django) or `unittest discover`
  - Ruby: `bin/rails test` / `bundle exec rspec` / `bundle exec rake test`
  - Extra arguments are passed through to the test runner, e.g. `dev unit ./internal/...`
  - Supports the same language flags as other commands (`--go`, `--php`, `--js`, `--python`, `--version`)

- **`dev prepare`** – Prepare the project for development:
  - Set `777` permissions on cache directories
  - Copy `.env.dist` / `.env.dev` to `.env`
  - Install/reinstall vendors (composer, npm, go mod, pip)

- **`dev dcr`** – Run `docker-compose up -d` and report running services

- **`dev recipe [name] [args...]`** – List and run reusable command sets from the recipe repository:
  - Without arguments: prints all recipes with descriptions
  - With a name: downloads (and caches) the recipe, prints its details and asks whether to run it
  - `--force` refreshes a cached recipe
  - See [Recipes](#recipes) for parameters and the repository configuration

- **`dev self-command`** – Open the custom commands config (`~/dev-command/custom.yml`) for editing:
  - Unknown `dev <name>` commands are matched against global and project-local (`.custom`) definitions
  - See [Custom Commands](#custom-commands) for the format and variables

- **`dev port <address>`** – Check if a port is occupied and show detailed process information:
  - Detects the listening process natively via `/proc` on Linux
  - Falls back to `lsof` on platforms without `/proc` (macOS)
  - Offers `nmap` scan for service detection
  - Supports remote hosts (auto‑runs nmap)
  - Formats: `127.0.0.1:1000`, `:8080`, `8080`

- **`dev curl <url> [method]`** – Make an HTTP request and interactively choose to display the response or save it to a file:
  - Automatically prepends `https://` if no protocol is specified
  - Uses `--insecure` mode (skips TLS certificate verification)
  - Methods: `GET` (default), `POST`, `PUT`, `DELETE`
  - Shows status, duration, and content length

- **`dev`** or **`dev analyze`** – Analyze current directory:
  - Detect language/framework (PHP, Go, Node.js, Python, etc.)
  - Check for `.env`, vendor installation, Docker services, Make commands
  - Detect databases (local, Docker, remote)
  - Colorful output with status indicators

- **`dev install [file]`** – Install the dev tool (or a specified executable) to a system directory (`/usr/local/bin`, `~/bin`, etc.) with interactive directory selection.

- **`dev self-update`** – Download and install the latest version of dev from GitHub releases.

- **`dev self-config`** – Open AI configuration file for editing:
  - Creates the file with default empty parameters if it doesn't exist
  - Uses `$EDITOR` or `nano` by default
  - Required parameters: `LLM_ENDPOINT`, `LLM_TOKEN`, `LLM_MODEL`

- **`dev virus [user:pass@ip_addr]`** – Copy the dev executable to a remote server via SCP (supports `user@host` or `user:pass@ip` formats). Automatically sets execute permissions.

- **`prod release`** – Production release management (prepare + switch):
  - `prod release prepare [name]` – copy build artifacts from `builds_folder` into a new `releases_folder/release-<datetime>` archive folder (source folder is left untouched); interactive release name selection (default: first) when omitted
  - `prod release switch [name]` – list the most recent releases (newest first, today's releases highlighted with a white background) and switch the `current_release_folder` symlink to the selected one; `-l/--lines` controls how many releases are shown (default: 5)
  - `prod release` – run both steps in order: prepare a new release, then switch to it
  - Configuration is read from `release.yml` in the current directory; if missing or invalid, an editor opens with a filled template (same behavior as `dev self-config`)
  - Per-release optional settings: `release_prefix` (default `release-`) and `release_time_format` (default `2006-01-02_15-04-05`)

- **`prod virus [user@ip_addr]`** – Copy the prod executable to a remote server via SCP and install the production configuration there:
  - `/etc/prod-command` config files (e.g. `deps.conf`; the `reports` history is not copied) — installed via `sudo` for non-root users
  - LLM config (`~/dev-command/main.conf` or `/etc/dev-command/main.conf`) so `prod llm` works right away
  - Supports `user@host` or just `ip` formats (SSH key auth only)

- **`prod service [--since 6h]`** – Show systemd services status:
  - User services (from `/etc/systemd/system`) and core services (nginx, php-fpm, postgresql, redis, etc., shown only if present in the system)
  - Color-coded state: red (failed/stopped), yellow (crashed in the period), white (restarted in the period), green (OK), with uptime in parentheses
  - Currently failed services (`systemctl --failed`)
  - Services that crashed in the period (journalctl error scan)
  - Interactive selection of a problem service to show its recent logs
  - `-s/--since` sets the look-back period (default: 6h, e.g. `2h`, `30m`)

- **`prod logs`** – Show and open server logs:
  - Numbered list: current folder (default), user services, core services
  - Selecting a service shows its journal plus standard log files (core: nginx access, postgres slow, etc.) or logs in the service working directory (user)
  - Files open in `lnav`; journal opens as a static snapshot of the last 200 lines in `lnav`

- **`prod install [file]`** – Install the prod tool (or a specified executable) to a system directory (`/usr/local/bin`, `~/bin`, etc.) with interactive directory selection.

- **`prod self-update`** – Download and install the latest version of prod from GitHub releases.

## Installation

### From GitHub

Linux / WSL:

```bash
wget -O dev https://github.com/ozonar/dev/releases/latest/download/dev-linux-amd64 && chmod +x dev && ./dev install -y; rm -f dev
```

macOS (Intel):

```bash
curl -L -o dev https://github.com/ozonar/dev/releases/latest/download/dev-darwin-amd64 && chmod +x dev && ./dev install -y; rm -f dev
```

macOS (Apple Silicon):

```bash
curl -L -o dev https://github.com/ozonar/dev/releases/latest/download/dev-darwin-arm64 && chmod +x dev && ./dev install -y; rm -f dev
```

Android (arm64):

```bash
wget -O dev https://github.com/ozonar/dev/releases/latest/download/dev-android-arm64 && chmod +x dev && ./dev install -y; rm -f dev
```

## Usage

Navigate to your project directory and run:

```bash
dev                     # analyze project
dev run                 # run project
dev run -p 8080         # run project on port 8080
dev run <file> -p 8080  # run a specific entry point on port 8080
dev build               # build project (auto-detect main package)
dev build ./cmd/prod    # build a specific package
dev review              # run static code analysis (dry-run)
dev review fix          # run analysis and auto-fix issues
dev review ai           # AI code review of the changed code
dev review ai --branch=main  # AI review of the diff with main (CI)
dev ai "install npm"    # ask AI to generate commands
dev db                  # interactive database explorer
dev logs                # show logs
dev migrate             # run database migrations
dev migrate status      # show migration status
dev migrate new         # create a new migration
dev cache               # clear cache
dev unit                # run unit tests
dev unit ./internal/... # run unit tests for specific packages
dev prepare             # list of prepare framework specific commands
dev dcr                 # start docker-compose
dev recipe              # list recipes from the recipe repository
dev recipe <name>       # run a recipe by name (downloaded on first use)
dev recipe --force <name>  # refresh a cached recipe and run it
dev self-command        # open custom commands config
dev port :8080          # check if port is occupied
dev curl example.com    # make HTTP request
dev install             # install dev to system
dev install <file>      # install file to system
dev self-update         # update dev to latest version
dev self-config         # configure AI settings
dev virus user@host     # copy dev to remote server

prod                    # production server health report
prod release            # prepare a new release and switch to it
prod release prepare    # copy build artifacts to releases/release-<datetime>
prod release switch -l 5  # switch the current release symlink
prod service            # show systemd services status
prod service -s 24h      # show services with a 2-hour crash window
prod logs               # show and open server logs
prod virus user@host    # copy prod to remote server
prod install            # install prod to system
prod self-update        # update prod to latest version
```

## Configuration

### AI Configuration

The AI feature requires configuration in `~/dev-command/main.conf`:

```ini
LLM_ENDPOINT=https://api.openai.com/v1/chat/completions
LLM_TOKEN=your-api-token
LLM_MODEL=gpt-4o
```

Use `dev self-config` to open the config file for editing.

### Recipes

Recipes are reusable command sets stored in a separate recipe repository.
`dev recipe` reads `repository.yml` from the repository root and prints all
recipe names with descriptions. `dev recipe <name>` reads the recipe file,
caches it in `~/dev-command/recipes`, prints its name, description and cache
time, and asks whether to run it. Use `--force` to refresh a cached recipe.

The repository is configured in `~/dev-command/main.conf`:

```ini
# Local path or HTTP(S) base URL.
RECIPE_REPO=https://raw.githubusercontent.com/ozonar/dev-recipe/main
```

The default is `https://raw.githubusercontent.com/ozonar/dev-recipe/main`.
Files are read from the local path or downloaded over HTTP.

A recipe may declare named parameters with a description and an optional
default value:

```yaml
commands:
  frpc:
    params:
      - name: LISTEN_PORT
        description: Local port to expose
        default: "8000"
    subcommands:
      - docker run --rm frp/frpc -l "$LISTEN_PORT"
```

Values come from a `NAME=value` argument, the environment, or an interactive
prompt, and fall back to the declared default:

```bash
dev recipe frpc LISTEN_PORT=9000
```

Parameters without a default are required. Extra arguments that are not
declared parameters are passed positionally as `$1`, `$2`, ... and are never
interpreted as shell syntax:

```bash
dev recipe frpc 8000
```

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

## AI Code Review in CI

`dev review ai` can be used as a quality gate. The command exits with code 1 when:

- the AI finds critical issues;
- the model does not return a verdict;
- the review itself fails (network, configuration, token).

LLM parameters are read from `~/dev-command/main.conf` or from environment variables. The environment wins, so no config file is needed on a runner:

| Variable | Where to store it |
|----------|-------------------|
| `LLM_ENDPOINT` | repository variable |
| `LLM_MODEL` | repository variable |
| `LLM_TOKEN` | secret |

The scope is selected with the same flags as `dev review`: `--code`, `--commit=N`, `--all`, or `--branch=NAME`. Pass a scope flag explicitly in CI: without it the command asks for the scope interactively. `--branch` accepts any existing base branch, for example `main`.

### GitHub Actions

A ready-to-use workflow is provided in `.github/workflows/ai-review.yml`. The same recipe works in any repository: install the `dev` binary from the release assets and pass the LLM credentials.

```yaml
name: AI Review
on:
  pull_request:
    branches: [ main ]
  push:
    branches: [ main ]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    env:
      LLM_ENDPOINT: ${{ vars.LLM_ENDPOINT }}
      LLM_MODEL: ${{ vars.LLM_MODEL }}
      LLM_TOKEN: ${{ secrets.LLM_TOKEN }}
    steps:
      - name: Check LLM token
        run: |
          if [ -z "$LLM_TOKEN" ]; then
            echo "::error::Set the LLM_TOKEN secret."
            exit 1
          fi
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # required for diff with the base branch
      - name: Install dev
        run: |
          curl -fsSL "https://github.com/ozonar/dev/releases/latest/download/dev-linux-amd64" -o "$RUNNER_TEMP/dev"
          chmod +x "$RUNNER_TEMP/dev"
      - name: AI review (pull request)
        if: github.event_name == 'pull_request'
        run: '"$RUNNER_TEMP/dev" review ai --branch=${{ github.base_ref }}'
      - name: AI review (push)
        if: github.event_name == 'push'
        run: '"$RUNNER_TEMP/dev" review ai --commit=1'
```

Store `LLM_ENDPOINT` and `LLM_MODEL` under Settings → Secrets and variables → Actions → Variables → Repository variables, and `LLM_TOKEN` under Settings → Secrets and variables → Actions → Secrets → Repository secrets. The workflow validates `LLM_TOKEN` and exits with code 1 when it is missing, so a misconfigured repository fails instead of reporting a green run.

### GitLab CI

```yaml
ai-review:
  image: golang:1.27.3
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    LLM_ENDPOINT: $LLM_ENDPOINT
    LLM_MODEL: $LLM_MODEL
    LLM_TOKEN: $LLM_TOKEN
    GIT_DEPTH: "0"   # required for diff with the base branch
  script:
    - |
      if [ -z "$LLM_TOKEN" ]; then
        echo "LLM_TOKEN is not set" >&2
        exit 1
      fi
    - curl -fsSL "https://github.com/ozonar/dev/releases/latest/download/dev-linux-amd64" -o /tmp/dev
    - chmod +x /tmp/dev
    - |
      if [ "$CI_PIPELINE_SOURCE" = "merge_request_event" ]; then
        /tmp/dev review ai --branch="$CI_MERGE_REQUEST_TARGET_BRANCH_NAME"
      else
        /tmp/dev review ai --commit=1
      fi
```

Store `LLM_TOKEN` as a masked CI/CD variable in Settings → CI/CD → Variables; `LLM_ENDPOINT` and `LLM_MODEL` may be plain variables. `GIT_DEPTH: "0"` disables shallow cloning so that `origin/<base branch>` is available for the diff.

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
│   ├── ai/config.go         # AI parameters (LLM keys)
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
│   ├── port/                # Port checking (native /proc, lsof fallback, nmap)
│   ├── prepare/             # Environment preparation
│   ├── procfs/              # Native process & socket inspection (/proc)
│   ├── prod/                # Production server health diagnostics
│   ├── recipe/              # Recipe repository, cache and runner
│   ├── release/             # Release management (release.yml, symlinks)
│   ├── run/                 # Project runner
│   ├── selfconfig/          # Shared main.conf paths, read and edit
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
