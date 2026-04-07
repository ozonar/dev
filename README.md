# Dev CLI Tool

A command-line tool to assist with development tasks: analyze projects, clear caches, view logs, run projects, manage Docker, and prepare environments.

## Features

- **`dev`** or **`dev analyze`** – Analyze current directory:
  - Detect language/framework (PHP, Go, Node.js, Python, etc.)
  - Check for `.env`, vendor installation, Docker services, Make commands
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

- **`dev run`** – Start the project with the appropriate runner:
  - Symfony: `symfony serve`
  - Laravel: `php artisan serve`
  - Go: `go run` (auto‑detects main files)
  - Node.js: `npm run dev`
  - Python: `python manage.py runserver` or simple HTTP server

- **`dev dcr`** – Run `docker‑compose up -d` and report running services

- **`dev prepare`** – Prepare the project for development:
  - Set `777` permissions on cache directories
  - Copy `.env.dist` / `.env.dev` to `.env`
  - Install/reinstall vendors (composer, npm, go mod, pip)

- **`dev install [file]`** – Install the dev tool (or a specified executable) to a system directory (`/usr/local/bin`, `~/bin`, etc.) with interactive directory selection.

- **`dev virus [user@ip_addr]`** – Copy the dev executable to a remote server via SCP (requires SSH key authentication). Automatically sets execute permissions.

- **`dev build`** – Build the project according to its language:
  - Go: detects main files, offers selection, builds executable
  - Node.js: runs `npm run build`
  - Other languages: no‑op (informs that building is not required)

## Installation

### From source

```bash
git clone <repository>
cd dev
go build -o dev ./cmd/dev
sudo mv dev /usr/local/bin/
```

### Using go install

```bash
go install dev
```

Ensure `$GOPATH/bin` is in your `PATH`.

## Usage

Navigate to your project directory and run:

```bash
dev                 # analyze project
dev cache           # clear cache
dev logs            # show logs
dev run             # run project
dev dcr             # start docker‑compose
dev prepare         # prepare environment
dev install         # install dev to system
dev virus user@host # copy dev to remote server
dev build           # build project
```

## Configuration

No configuration files are required. The tool automatically detects your project based on common file patterns.

## Dependencies

- Go 1.20+
- Docker & docker‑compose (optional, for `dev dcr`)
- lnav (optional, for `dev logs` interactive viewing)
- Framework‑specific tools (php, npm, go, python, etc.)
- SSH keys (for `dev virus`)

## Project Structure

```
dev/
├── cmd/dev/main.go          # CLI entry point
├── internal/
│   ├── detector/            # Project detection
│   ├── cache/               # Cache clearing
│   ├── logs/                # Log discovery
│   ├── run/                 # Project runner
│   ├── docker/              # Docker‑compose operations
│   ├── prepare/             # Environment preparation
│   ├── install/             # Installation logic
│   ├── virus/               # Remote copy via SCP
│   ├── build/               # Project building
│   └── version/             # Version information
├── go.mod
└── README.md
```

## License

CC-BY-NC-4.0