package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dev/internal/detector"

	"github.com/fatih/color"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// DBType тип базы данных
type DBType string

const (
	DBPostgres DBType = "postgresql"
	DBMySQL    DBType = "mysql"
)

// MigrationStatus содержит полную информацию о состоянии миграции
type MigrationStatus struct {
	// Общий статус
	Status   string // RUNNING, IDLE, WAITING_LOCK, WAITING_IO, FAILED, COMPLETED, UNKNOWN
	Phase    string // SQL EXECUTION, PHP PROCESSING, IDLE
	Duration string // время выполнения

	// PHP процесс
	PHP struct {
		PID     int
		Started string // время старта процесса (HH:MM:SS)
		Runtime string // время работы процесса
		CPU     string
		Memory  string
		Stat    string
		Wchan   string
		Command string
		Exists  bool
	}

	// База данных
	DB struct {
		PID          int
		State        string
		Transaction  string
		QueryRuntime string
		WaitEvent    string
		Query        string
		User         string
		Client       string
		Exists       bool
	}

	// Doctrine миграции
	Doctrine struct {
		Executed int
		Pending  int
		Current  string
		Previous string
		Next     string
	}

	// Блокировки
	Locks struct {
		Blocked bool
		Chain   []LockChainLink
	}

	// Диагноз
	Diagnosis string
	Action    string
}

// LockChainLink представляет одно звено в цепочке блокировок
type LockChainLink struct {
	PID      int
	Query    string
	Duration string
	User     string
	AppName  string
}

// RunMigrationStatus выполняет анализ статуса миграции Doctrine
func RunMigrationStatus() error {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		return fmt.Errorf("error detecting project: %v", err)
	}

	// Находим базу данных (PostgreSQL или MySQL)
	var dbInfo *detector.DatabaseInfo
	for _, db := range info.Databases {
		if db.Type == "postgresql" || db.Type == "mysql" {
			dbInfo = &db
			break
		}
	}

	if dbInfo == nil {
		return fmt.Errorf("no PostgreSQL or MySQL database found in project configuration")
	}

	status := &MigrationStatus{}

	// 1. Ищем PHP процесс миграции Doctrine
	findPHPMigrationProcess(status)

	// 2. Если нашли PHP процесс, ищем соответствующий процесс в БД
	if status.PHP.Exists {
		findDBProcess(status, dbInfo)
	}

	// 3. Анализируем блокировки
	if status.DB.Exists {
		analyzeLocks(status, dbInfo)
	}

	// 4. Получаем информацию о Doctrine миграциях
	getDoctrineMigrationInfo(status, dbInfo)

	// 5. Формируем диагноз
	generateDiagnosis(status)

	// 6. Выводим результат
	printMigrationStatus(status, dbInfo)

	return nil
}

// findPHPMigrationProcess ищет PHP процесс, выполняющий doctrine:migrations:migrate
func findPHPMigrationProcess(status *MigrationStatus) {
	// Ищем PHP процесс с doctrine:migrations:migrate
	cmd := exec.Command("sh", "-c", `ps aux | grep -E '[d]octrine.*migrations.*migrate|[a]rtisan.*migrate|[b]in/console.*migrate' | head -1`)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		// Пробуем найти любой PHP процесс, выполняющий миграции
		cmd = exec.Command("sh", "-c", `ps aux | grep -E '[p]hp.*migrate' | head -1`)
		output, err = cmd.Output()
		if err != nil || len(output) == 0 {
			status.Status = "IDLE"
			status.Phase = "IDLE"
			status.Diagnosis = "No active migration process found."
			status.Action = "Run migration to start the process."
			return
		}
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		status.Status = "IDLE"
		return
	}

	fields := strings.Fields(line)
	if len(fields) < 11 {
		return
	}

	// ps aux format: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return
	}

	status.PHP.PID = pid
	status.PHP.CPU = fields[2]
	status.PHP.Memory = fields[3]
	status.PHP.Stat = fields[7]
	status.PHP.Command = strings.Join(fields[10:], " ")
	status.PHP.Exists = true

	// Получаем детальную информацию через ps -o
	// lstart — полное время старта, etime — elapsed time
	psCmd := exec.Command("ps", "-o", "pid,lstart,etime,%cpu,%mem,stat,wchan:32,cmd", "-p", strconv.Itoa(pid))
	psOutput, err := psCmd.Output()
	if err == nil {
		psLines := strings.Split(strings.TrimSpace(string(psOutput)), "\n")
		if len(psLines) >= 2 {
			psFields := strings.Fields(psLines[1])
			if len(psFields) >= 8 {
				// lstart format: Mon DD HH:MM:SS YYYY — берём HH:MM:SS
				status.PHP.Started = psFields[3] // HH:MM:SS
				status.PHP.Runtime = psFields[5] // etime
				status.PHP.CPU = psFields[6]
				status.PHP.Memory = psFields[7]
				status.PHP.Stat = psFields[8]
				status.PHP.Wchan = psFields[9]
			}
		}
	}

	// Определяем статус на основе WCHAN и CPU
	cpuVal, _ := strconv.ParseFloat(status.PHP.CPU, 64)
	if cpuVal > 10 {
		status.Status = "RUNNING"
		status.Phase = "SQL EXECUTION"
	} else if status.PHP.Wchan == "futex" {
		status.Status = "WAITING_LOCK"
		status.Phase = "WAITING FOR LOCK"
	} else if status.PHP.Wchan == "poll" || status.PHP.Wchan == "poll_schedule_timeout" {
		status.Status = "WAITING_IO"
		status.Phase = "WAITING FOR I/O"
	} else {
		status.Status = "RUNNING"
		status.Phase = "PHP PROCESSING"
	}
}

// findDBProcess находит процесс в БД, связанный с PHP миграцией
func findDBProcess(status *MigrationStatus, dbInfo *detector.DatabaseInfo) {
	dsn := buildDSN(dbInfo)
	if dsn == "" {
		return
	}

	driver := "postgres"
	if dbInfo.Type == "mysql" {
		driver = "mysql"
	}

	database, err := sql.Open(driver, dsn)
	if err != nil {
		return
	}
	defer database.Close()
	database.SetConnMaxLifetime(5 * time.Second)

	if dbInfo.Type == "postgresql" {
		findPostgresDBProcess(status, database)
	} else if dbInfo.Type == "mysql" {
		findMySQLDBProcess(status, database)
	}
}

// findPostgresDBProcess находит активный PostgreSQL процесс
func findPostgresDBProcess(status *MigrationStatus, db *sql.DB) {
	// Ищем активные DML запросы
	query := `
		SELECT 
			pid,
			state,
			query,
			EXTRACT(EPOCH FROM (NOW() - query_start))::integer AS query_runtime_seconds,
			EXTRACT(EPOCH FROM (NOW() - xact_start))::integer AS transaction_seconds,
			COALESCE(wait_event, 'none') AS wait_event,
			usename,
			COALESCE(client_addr::text, '') AS client_addr,
			COALESCE(application_name, '') AS application_name
		FROM pg_stat_activity
		WHERE (query ILIKE '%DELETE%' OR query ILIKE '%INSERT%' OR query ILIKE '%UPDATE%' 
			OR query ILIKE '%ALTER%' OR query ILIKE '%CREATE%' OR query ILIKE '%DROP%' 
			OR query ILIKE '%migration%' OR query ILIKE '%TRUNCATE%')
			AND state = 'active'
			AND pid <> pg_backend_pid()
			AND datname = current_database()
		ORDER BY query_start DESC
		LIMIT 1
	`

	row := db.QueryRow(query)
	var pid int
	var state, q, waitEvent, usename, clientAddr, appName string
	var queryRuntimeSec, transactionSec *int

	err := row.Scan(&pid, &state, &q, &queryRuntimeSec, &transactionSec, &waitEvent, &usename, &clientAddr, &appName)
	if err != nil {
		// Если не нашли активный DML запрос, ищем любой активный запрос
		query = `
			SELECT 
				pid,
				state,
				query,
				EXTRACT(EPOCH FROM (NOW() - query_start))::integer AS query_runtime_seconds,
				EXTRACT(EPOCH FROM (NOW() - xact_start))::integer AS transaction_seconds,
				COALESCE(wait_event, 'none') AS wait_event,
				usename,
				COALESCE(client_addr::text, '') AS client_addr,
				COALESCE(application_name, '') AS application_name
			FROM pg_stat_activity
			WHERE state = 'active'
				AND pid <> pg_backend_pid()
				AND datname = current_database()
			ORDER BY query_start DESC
			LIMIT 1
		`
		row = db.QueryRow(query)
		err = row.Scan(&pid, &state, &q, &queryRuntimeSec, &transactionSec, &waitEvent, &usename, &clientAddr, &appName)
		if err != nil {
			return
		}
	}

	status.DB.PID = pid
	status.DB.State = state
	status.DB.Query = q
	status.DB.WaitEvent = waitEvent
	status.DB.User = usename
	status.DB.Client = clientAddr
	status.DB.Exists = true

	if queryRuntimeSec != nil {
		status.DB.QueryRuntime = formatDuration(*queryRuntimeSec)
	}
	if transactionSec != nil {
		status.DB.Transaction = formatDuration(*transactionSec)
		status.Duration = status.DB.Transaction
	}

	// Обновляем статус на основе wait_event
	if waitEvent != "none" && waitEvent != "" {
		if strings.Contains(waitEvent, "lock") || strings.Contains(waitEvent, "Lock") {
			status.Status = "WAITING_LOCK"
			status.Phase = "WAITING FOR LOCK"
		} else {
			status.Status = "WAITING_IO"
			status.Phase = fmt.Sprintf("WAITING FOR %s", waitEvent)
		}
	} else if status.Status != "WAITING_LOCK" {
		status.Status = "RUNNING"
		status.Phase = "SQL EXECUTION"
	}
}

// findMySQLDBProcess находит активный MySQL процесс
func findMySQLDBProcess(status *MigrationStatus, db *sql.DB) {
	// Ищем активные запросы (не наш собственный)
	query := `
		SELECT 
			ID,
			COMMAND,
			STATE,
			INFO,
			TIME,
			TIME as transaction_seconds,
			USER,
			HOST,
			DB
		FROM INFORMATION_SCHEMA.PROCESSLIST
		WHERE COMMAND != 'Sleep'
			AND ID <> CONNECTION_ID()
			AND DB = DATABASE()
		ORDER BY TIME DESC
		LIMIT 1
	`

	row := db.QueryRow(query)
	var pid int
	var command, state, info, user, host, databaseName string
	var timeSec, transactionSec int

	err := row.Scan(&pid, &command, &state, &info, &timeSec, &transactionSec, &user, &host, &databaseName)
	if err != nil {
		// Если не нашли, ищем любой процесс включая Sleep
		query = `
			SELECT 
				ID,
				COMMAND,
				STATE,
				INFO,
				TIME,
				TIME as transaction_seconds,
				USER,
				HOST,
				DB
			FROM INFORMATION_SCHEMA.PROCESSLIST
			WHERE ID <> CONNECTION_ID()
				AND DB = DATABASE()
			ORDER BY TIME DESC
			LIMIT 1
		`
		row = db.QueryRow(query)
		err = row.Scan(&pid, &command, &state, &info, &timeSec, &transactionSec, &user, &host, &databaseName)
		if err != nil {
			return
		}
	}

	status.DB.PID = pid
	status.DB.State = state
	status.DB.Query = info
	status.DB.User = user
	status.DB.Client = host
	status.DB.Exists = true

	status.DB.QueryRuntime = formatDuration(timeSec)
	status.DB.Transaction = formatDuration(transactionSec)
	status.Duration = status.DB.Transaction

	// MySQL wait events через Performance Schema (если доступно)
	waitQuery := `
		SELECT EVENT_NAME
		FROM performance_schema.events_waits_current
		WHERE THREAD_ID = (SELECT THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID = ?)
		LIMIT 1
	`
	var eventName string
	err = db.QueryRow(waitQuery, pid).Scan(&eventName)
	if err == nil && eventName != "" {
		status.DB.WaitEvent = eventName
		if strings.Contains(eventName, "lock") || strings.Contains(eventName, "Lock") {
			status.Status = "WAITING_LOCK"
			status.Phase = "WAITING FOR LOCK"
		} else {
			status.Status = "WAITING_IO"
			status.Phase = fmt.Sprintf("WAITING FOR %s", eventName)
		}
	} else {
		status.DB.WaitEvent = "none"
		if status.Status != "WAITING_LOCK" {
			status.Status = "RUNNING"
			status.Phase = "SQL EXECUTION"
		}
	}
}

// analyzeLocks анализирует цепочку блокировок
func analyzeLocks(status *MigrationStatus, dbInfo *detector.DatabaseInfo) {
	dsn := buildDSN(dbInfo)
	if dsn == "" {
		return
	}

	driver := "postgres"
	if dbInfo.Type == "mysql" {
		driver = "mysql"
	}

	database, err := sql.Open(driver, dsn)
	if err != nil {
		return
	}
	defer database.Close()
	database.SetConnMaxLifetime(5 * time.Second)

	if dbInfo.Type == "postgresql" {
		analyzePostgresLocks(status, database)
	} else if dbInfo.Type == "mysql" {
		analyzeMySQLLocks(status, database)
	}
}

// analyzePostgresLocks анализирует блокировки PostgreSQL
func analyzePostgresLocks(status *MigrationStatus, db *sql.DB) {
	lockQuery := `
		WITH recursive lock_chain AS (
			SELECT
				blocked.pid AS blocked_pid,
				blocking.pid AS blocking_pid,
				blocked.query AS blocked_query,
				blocking.query AS blocking_query,
				blocking.usename AS blocking_user,
				COALESCE(blocking.application_name, '') AS blocking_app,
				EXTRACT(EPOCH FROM (NOW() - blocking.query_start))::integer AS blocking_duration,
				1 AS level
			FROM pg_stat_activity blocked
			JOIN pg_locks blocked_locks ON blocked.pid = blocked_locks.pid
			JOIN pg_locks blocking_locks
				ON blocking_locks.locktype = blocked_locks.locktype
				AND blocking_locks.database IS NOT DISTINCT FROM blocked_locks.database
				AND blocking_locks.relation IS NOT DISTINCT FROM blocked_locks.relation
				AND blocking_locks.page IS NOT DISTINCT FROM blocked_locks.page
				AND blocking_locks.tuple IS NOT DISTINCT FROM blocked_locks.tuple
				AND blocking_locks.virtualxid IS NOT DISTINCT FROM blocked_locks.virtualxid
				AND blocking_locks.transactionid IS NOT DISTINCT FROM blocked_locks.transactionid
				AND blocking_locks.classid IS NOT DISTINCT FROM blocked_locks.classid
				AND blocking_locks.objid IS NOT DISTINCT FROM blocked_locks.objid
				AND blocking_locks.objsubid IS NOT DISTINCT FROM blocked_locks.objsubid
				AND blocking_locks.granted
			JOIN pg_stat_activity blocking ON blocking.pid = blocking_locks.pid
			WHERE NOT blocked_locks.granted

			UNION ALL

			SELECT
				lc.blocking_pid,
				blocking.pid,
				lc.blocking_query,
				blocking.query,
				blocking.usename,
				COALESCE(blocking.application_name, ''),
				EXTRACT(EPOCH FROM (NOW() - blocking.query_start))::integer,
				lc.level + 1
			FROM lock_chain lc
			JOIN pg_stat_activity blocked ON blocked.pid = lc.blocking_pid
			JOIN pg_locks blocked_locks ON blocked.pid = blocked_locks.pid
			JOIN pg_locks blocking_locks
				ON blocking_locks.locktype = blocked_locks.locktype
				AND blocking_locks.database IS NOT DISTINCT FROM blocked_locks.database
				AND blocking_locks.relation IS NOT DISTINCT FROM blocked_locks.relation
				AND blocking_locks.page IS NOT DISTINCT FROM blocked_locks.page
				AND blocking_locks.tuple IS NOT DISTINCT FROM blocked_locks.tuple
				AND blocking_locks.virtualxid IS NOT DISTINCT FROM blocked_locks.virtualxid
				AND blocking_locks.transactionid IS NOT DISTINCT FROM blocked_locks.transactionid
				AND blocking_locks.classid IS NOT DISTINCT FROM blocked_locks.classid
				AND blocking_locks.objid IS NOT DISTINCT FROM blocked_locks.objid
				AND blocking_locks.objsubid IS NOT DISTINCT FROM blocked_locks.objsubid
				AND blocking_locks.granted
			JOIN pg_stat_activity blocking ON blocking.pid = blocking_locks.pid
			WHERE NOT blocked_locks.granted
				AND lc.level < 10
		)
		SELECT DISTINCT
			blocked_pid,
			blocking_pid,
			blocked_query,
			blocking_query,
			blocking_user,
			blocking_app,
			blocking_duration
		FROM lock_chain
		ORDER BY level
	`

	rows, err := db.Query(lockQuery)
	if err != nil {
		return
	}
	defer rows.Close()

	buildLockChain(status, rows)
}

// analyzeMySQLLocks анализирует блокировки MySQL
func analyzeMySQLLocks(status *MigrationStatus, db *sql.DB) {
	// MySQL 8.0+ использует performance_schema.data_locks
	lockQuery := `
		SELECT
			r.PROCESSLIST_ID AS blocked_pid,
			b.PROCESSLIST_ID AS blocking_pid,
			r.PROCESSLIST_INFO AS blocked_query,
			b.PROCESSLIST_INFO AS blocking_query,
			b.PROCESSLIST_USER AS blocking_user,
			COALESCE(b.PROCESSLIST_HOST, '') AS blocking_host,
			TIMESTAMPDIFF(SECOND, b.PROCESSLIST_TIME, NOW()) AS blocking_duration
		FROM performance_schema.data_locks l
		JOIN performance_schema.threads r_t ON l.THREAD_ID = r_t.THREAD_ID
		JOIN information_schema.processlist r ON r_t.PROCESSLIST_ID = r.ID
		JOIN performance_schema.data_lock_waits w 
			ON l.ENGINE_LOCK_ID = w.REQUESTING_ENGINE_LOCK_ID
		JOIN performance_schema.threads b_t ON w.BLOCKING_THREAD_ID = b_t.THREAD_ID
		JOIN information_schema.processlist b ON b_t.PROCESSLIST_ID = b.ID
	`

	rows, err := db.Query(lockQuery)
	if err != nil {
		// Если performance_schema недоступен, пробуем SHOW PROCESSLIST и INNODB_TRX
		fallbackQuery := `
			SELECT
				trx.trx_mysql_thread_id AS blocked_pid,
				blocking_trx.trx_mysql_thread_id AS blocking_pid,
				blocked_pr.INFO AS blocked_query,
				blocking_pr.INFO AS blocking_query,
				blocking_pr.USER AS blocking_user,
				COALESCE(blocking_pr.HOST, '') AS blocking_host,
				TIMESTAMPDIFF(SECOND, blocking_trx.trx_started, NOW()) AS blocking_duration
			FROM information_schema.innodb_trx trx
			JOIN information_schema.innodb_lock_waits w ON trx.trx_id = w.requesting_trx_id
			JOIN information_schema.innodb_trx blocking_trx ON w.blocking_trx_id = blocking_trx.trx_id
			JOIN information_schema.processlist blocked_pr ON trx.trx_mysql_thread_id = blocked_pr.ID
			JOIN information_schema.processlist blocking_pr ON blocking_trx.trx_mysql_thread_id = blocking_pr.ID
		`
		rows, err = db.Query(fallbackQuery)
		if err != nil {
			return
		}
	}
	defer rows.Close()

	buildLockChain(status, rows)
}

// buildLockChain строит цепочку блокировок из результатов запроса
func buildLockChain(status *MigrationStatus, rows *sql.Rows) {
	var chain []LockChainLink
	seen := make(map[int]bool)

	for rows.Next() {
		var blockedPID, blockingPID, blockingDuration int
		var blockedQuery, blockingQuery, blockingUser, blockingApp string

		err := rows.Scan(&blockedPID, &blockingPID, &blockedQuery, &blockingQuery,
			&blockingUser, &blockingApp, &blockingDuration)
		if err != nil {
			continue
		}

		if !seen[blockedPID] {
			chain = append(chain, LockChainLink{
				PID:      blockedPID,
				Query:    blockedQuery,
				Duration: formatDuration(blockingDuration),
				User:     blockingUser,
				AppName:  blockingApp,
			})
			seen[blockedPID] = true
		}

		if !seen[blockingPID] {
			chain = append(chain, LockChainLink{
				PID:      blockingPID,
				Query:    blockingQuery,
				Duration: formatDuration(blockingDuration),
				User:     blockingUser,
				AppName:  blockingApp,
			})
			seen[blockingPID] = true
		}
	}

	if len(chain) > 0 {
		status.Locks.Blocked = true
		status.Locks.Chain = chain
	}
}

// getDoctrineMigrationInfo получает информацию о миграциях Doctrine из БД
func getDoctrineMigrationInfo(status *MigrationStatus, dbInfo *detector.DatabaseInfo) {
	dsn := buildDSN(dbInfo)
	if dsn == "" {
		return
	}

	driver := "postgres"
	if dbInfo.Type == "mysql" {
		driver = "mysql"
	}

	database, err := sql.Open(driver, dsn)
	if err != nil {
		return
	}
	defer database.Close()
	database.SetConnMaxLifetime(5 * time.Second)

	// Проверяем наличие таблицы doctrine_migration_versions
	var tableExists bool
	if dbInfo.Type == "postgresql" {
		err = database.QueryRow(`SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'doctrine_migration_versions'
		)`).Scan(&tableExists)
	} else {
		err = database.QueryRow(`SELECT COUNT(*) > 0 
			FROM information_schema.tables 
			WHERE table_name = 'doctrine_migration_versions'
			AND table_schema = DATABASE()`).Scan(&tableExists)
	}
	if err != nil || !tableExists {
		return
	}

	// Получаем общее количество миграций
	err = database.QueryRow(`SELECT COUNT(*) FROM doctrine_migration_versions`).Scan(&status.Doctrine.Executed)
	if err != nil {
		status.Doctrine.Executed = 0
	}

	// Получаем текущую (последнюю выполненную) миграцию
	var currentVersion string
	err = database.QueryRow(`
		SELECT version
		FROM doctrine_migration_versions
		ORDER BY executed_at DESC
		LIMIT 1
	`).Scan(&currentVersion)
	if err == nil {
		status.Doctrine.Current = normalizeVersion(currentVersion)
	}

	// Получаем предыдущую миграцию
	var prevVersion string
	err = database.QueryRow(`
		SELECT version
		FROM doctrine_migration_versions
		ORDER BY executed_at DESC
		OFFSET 1
		LIMIT 1
	`).Scan(&prevVersion)
	if err == nil {
		status.Doctrine.Previous = normalizeVersion(prevVersion)
	}

	// Определяем количество pending миграций
	pendingCount := findPendingMigrations(status.Doctrine.Current)
	status.Doctrine.Pending = pendingCount
	if pendingCount > 0 {
		status.Doctrine.Next = findNextMigrationVersion(status.Doctrine.Current)
	}
}

// findPendingMigrations ищет файлы миграций, которые ещё не были выполнены
func findPendingMigrations(currentVersion string) int {
	dirs := []string{
		"migrations",
		"db/migrations",
		"app/DoctrineMigrations",
		"src/Migrations",
	}

	re := regexp.MustCompile(`Version(\d{14})`)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		pending := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			matches := re.FindStringSubmatch(entry.Name())
			if len(matches) >= 2 {
				version := "Version" + matches[1]
				if version > currentVersion {
					pending++
				}
			}
		}
		return pending
	}

	return 0
}

// findNextMigrationVersion находит следующую версию миграции после текущей
func findNextMigrationVersion(currentVersion string) string {
	dirs := []string{
		"migrations",
		"db/migrations",
		"app/DoctrineMigrations",
		"src/Migrations",
	}

	re := regexp.MustCompile(`Version(\d{14})`)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		var nextVersion string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			matches := re.FindStringSubmatch(entry.Name())
			if len(matches) >= 2 {
				version := "Version" + matches[1]
				if version > currentVersion {
					if nextVersion == "" || version < nextVersion {
						nextVersion = version
					}
				}
			}
		}
		return nextVersion
	}

	return ""
}

// normalizeVersion извлекает короткое имя версии из полного FQCN
// Doctrine хранит версии как "DoctrineMigrations\Version20260719041152"
// Нормализуем до "Version20260719041152"
func normalizeVersion(version string) string {
	re := regexp.MustCompile(`(Version\d{14})`)
	matches := re.FindStringSubmatch(version)
	if len(matches) >= 2 {
		return matches[1]
	}
	return version
}

// generateDiagnosis формирует диагноз на основе собранных данных
func generateDiagnosis(status *MigrationStatus) {
	switch status.Status {
	case "RUNNING":
		if status.Phase == "SQL EXECUTION" {
			status.Diagnosis = "Migration is actively executing SQL."
			if status.DB.State == "active" {
				status.Diagnosis += "\nDatabase backend is processing the query."
			}
			status.Action = "Wait for the migration to complete."
		} else {
			status.Diagnosis = "Migration PHP process is running."
			status.Action = "Monitor the process."
		}

	case "WAITING_LOCK":
		status.Diagnosis = "Migration is waiting for a database lock."
		if len(status.Locks.Chain) > 0 {
			status.Diagnosis += "\n\nBlocking chain detected:"
			for i, link := range status.Locks.Chain {
				status.Diagnosis += fmt.Sprintf("\n  PID %d", link.PID)
				if i < len(status.Locks.Chain)-1 {
					status.Diagnosis += " →"
				}
			}
		}
		status.Action = "Inspect the blocking transaction before terminating it."

	case "WAITING_IO":
		status.Diagnosis = "Migration is waiting for I/O operations."
		if status.DB.WaitEvent != "" && status.DB.WaitEvent != "none" {
			status.Diagnosis += fmt.Sprintf("\nWait event: %s", status.DB.WaitEvent)
		}
		status.Action = "Check disk I/O and system resources."

	case "IDLE":
		status.Diagnosis = "No active migration process found."
		status.Action = "Run migration to start the process."

	default:
		status.Diagnosis = "Migration status is unknown."
		status.Action = "Investigate manually."
	}

	// Дополнительные проверки
	if status.PHP.Exists && !status.DB.Exists {
		status.Diagnosis = "Migration process exists, but no active database query was found.\n\nPossible causes:\n- PHP is waiting outside database\n- Doctrine is processing data\n- Connection is idle in transaction\n- Process is deadlocked internally"
		status.Action = "Inspect PHP process state with strace or gdb."
	}
}

// printMigrationStatus выводит отформатированный статус миграции
func printMigrationStatus(status *MigrationStatus, dbInfo *detector.DatabaseInfo) {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	white := color.New(color.FgWhite).SprintFunc()
	magenta := color.New(color.FgMagenta).SprintFunc()

	// Определяем тип БД для отображения
	dbTypeDisplay := "PostgreSQL"
	if dbInfo.Type == "mysql" {
		dbTypeDisplay = "MySQL"
	}

	// Заголовок
	fmt.Println()
	color.Cyan("Migration status")
	fmt.Println(strings.Repeat("─", 62))

	// Блок: Database backup info
	fmt.Println()
	color.Cyan("Database backup info")
	fmt.Printf("  Connection: %s %s\n", cyan(dbTypeDisplay), cyan(getDBVersion(dbInfo)))
	if status.Doctrine.Current != "" {
		fmt.Printf("  Migration:  %s\n", yellow(status.Doctrine.Current))
	}
	fmt.Printf("  Command:    %s\n", white("doctrine:migrations:migrate"))
	if status.PHP.Started != "" {
		fmt.Printf("  Started:    %s\n", yellow(status.PHP.Started))
	}
	if status.Duration != "" {
		fmt.Printf("  Duration:   %s\n", yellow(status.Duration))
	}

	// Статус
	fmt.Println()
	color.Cyan("Status")
	var statusColor func(a ...interface{}) string
	switch status.Status {
	case "RUNNING":
		statusColor = green
	case "WAITING_LOCK":
		statusColor = red
	case "WAITING_IO":
		statusColor = yellow
	case "IDLE":
		statusColor = white
	default:
		statusColor = yellow
	}
	fmt.Printf("  Status:  %s\n", statusColor(status.Status))
	fmt.Printf("  Phase:   %s\n", cyan(status.Phase))
	if status.Duration != "" {
		fmt.Printf("  Duration: %s\n", yellow(status.Duration))
	}

	// Текущий запрос
	if status.DB.Query != "" {
		fmt.Println()
		color.Cyan("Current query")
		query := formatSQLQuery(status.DB.Query)
		fmt.Printf("  %s\n", white(query))
	}

	// Процесс миграции
	if status.PHP.Exists {
		fmt.Println()
		color.Cyan("Migration process")
		fmt.Printf("  PID:     %s\n", yellow(strconv.Itoa(status.PHP.PID)))
		if status.PHP.Started != "" {
			fmt.Printf("  Started: %s\n", yellow(status.PHP.Started))
		}
		if status.PHP.Runtime != "" {
			fmt.Printf("  Runtime: %s\n", yellow(status.PHP.Runtime))
		}
		if status.PHP.CPU != "" {
			fmt.Printf("  CPU:     %s%%\n", yellow(status.PHP.CPU))
		}
		if status.PHP.Memory != "" {
			fmt.Printf("  Memory:  %s MB\n", yellow(status.PHP.Memory))
		}
		if status.PHP.Stat != "" {
			fmt.Printf("  STAT:    %s\n", yellow(status.PHP.Stat))
		}
		if status.PHP.Wchan != "" {
			fmt.Printf("  WCHAN:   %s\n", yellow(status.PHP.Wchan))
		}
	}

	// Соединение с БД
	if status.DB.Exists {
		fmt.Println()
		color.Cyan("Database process")
		fmt.Printf("  PID:          %s\n", yellow(strconv.Itoa(status.DB.PID)))
		fmt.Printf("  State:        %s\n", yellow(status.DB.State))
		if status.DB.Transaction != "" {
			fmt.Printf("  Transaction:  %s\n", yellow(status.DB.Transaction))
		}
		if status.DB.QueryRuntime != "" {
			fmt.Printf("  Query runtime: %s\n", yellow(status.DB.QueryRuntime))
		}
		fmt.Printf("  Wait event:   %s\n", yellow(status.DB.WaitEvent))
		if status.DB.User != "" {
			fmt.Printf("  DB user:      %s\n", yellow(status.DB.User))
		}
		if status.DB.Client != "" {
			fmt.Printf("  Client:       %s\n", yellow(status.DB.Client))
		}
	}

	// Блокировки
	fmt.Println()
	color.Cyan("Locks")
	if status.Locks.Blocked {
		fmt.Printf("  Status: %s\n", red("BLOCKED"))
		fmt.Println()
		fmt.Printf("  %s\n", magenta("Blocking chain:"))
		for i, link := range status.Locks.Chain {
			prefix := "  └─"
			if i < len(status.Locks.Chain)-1 {
				prefix = "  ├─"
			}
			fmt.Printf("  %s PID %s\n", prefix, red(strconv.Itoa(link.PID)))
			if link.Query != "" {
				shortQuery := link.Query
				if len(shortQuery) > 100 {
					shortQuery = shortQuery[:100] + "..."
				}
				fmt.Printf("  %s   Query: %s\n", prefix, white(shortQuery))
			}
			if link.Duration != "" {
				fmt.Printf("  %s   Duration: %s\n", prefix, yellow(link.Duration))
			}
			if link.User != "" {
				fmt.Printf("  %s   User: %s\n", prefix, yellow(link.User))
			}
			if i < len(status.Locks.Chain)-1 {
				fmt.Printf("  %s   ↓ waits for\n", prefix)
			}
		}
	} else {
		fmt.Printf("  Status: %s\n", green("NOT BLOCKED"))
	}

	// Doctrine migration status
	if status.Doctrine.Executed > 0 || status.Doctrine.Pending > 0 {
		fmt.Println()
		color.Cyan("Doctrine migrations")
		fmt.Printf("  Executed: %s\n", green(strconv.Itoa(status.Doctrine.Executed)))
		fmt.Printf("  Available: %s\n", cyan(strconv.Itoa(status.Doctrine.Executed+status.Doctrine.Pending)))
		fmt.Printf("  Pending:  %s\n", yellow(strconv.Itoa(status.Doctrine.Pending)))
		fmt.Println()
		if status.Doctrine.Current != "" {
			fmt.Printf("  Current:\n    %s\n", yellow(status.Doctrine.Current))
		}
		if status.Doctrine.Previous != "" {
			fmt.Printf("  Previous:\n    %s\n", white(status.Doctrine.Previous))
		}
		if status.Doctrine.Next != "" {
			fmt.Printf("  Next:\n    %s\n", green(status.Doctrine.Next))
		}
	}

	// Diagnosis
	fmt.Println()
	color.Cyan("Diagnosis")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  %s\n", white(status.Diagnosis))
	fmt.Println()
	fmt.Printf("  %s: %s\n", magenta("Action"), yellow(status.Action))
	fmt.Println()
}

// getDBVersion получает версию БД
func getDBVersion(dbInfo *detector.DatabaseInfo) string {
	dsn := buildDSN(dbInfo)
	if dsn == "" {
		return ""
	}

	driver := "postgres"
	if dbInfo.Type == "mysql" {
		driver = "mysql"
	}

	database, err := sql.Open(driver, dsn)
	if err != nil {
		return ""
	}
	defer database.Close()
	database.SetConnMaxLifetime(3 * time.Second)

	var version string
	if dbInfo.Type == "postgresql" {
		if err := database.QueryRow("SELECT version()").Scan(&version); err != nil {
			return ""
		}
		// Извлекаем номер версии
		re := regexp.MustCompile(`PostgreSQL (\d+)`)
		matches := re.FindStringSubmatch(version)
		if len(matches) >= 2 {
			return matches[1]
		}
	} else {
		if err := database.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
			return ""
		}
		re := regexp.MustCompile(`(\d+\.\d+\.\d+)`)
		matches := re.FindStringSubmatch(version)
		if len(matches) >= 2 {
			return matches[1]
		}
	}

	return ""
}

// buildDSN строит DSN для подключения к БД из DatabaseInfo
func buildDSN(dbInfo *detector.DatabaseInfo) string {
	if dbInfo == nil {
		return ""
	}

	host := dbInfo.Host
	if host == "" {
		host = "localhost"
	}
	port := dbInfo.Port
	if port == "" {
		if dbInfo.Type == "mysql" {
			port = "3306"
		} else {
			port = "5432"
		}
	}
	dbname := dbInfo.Database
	if dbname == "" {
		dbname = "postgres"
	}

	if dbInfo.Type == "mysql" {
		// MySQL DSN: user:password@tcp(host:port)/dbname?parseTime=true
		// Извлекаем user и password из URL если есть
		user := "root"
		password := ""
		if dbInfo.URL != "" {
			re := regexp.MustCompile(`mysql://([^:]+):([^@]+)@`)
			matches := re.FindStringSubmatch(dbInfo.URL)
			if len(matches) >= 3 {
				user = matches[1]
				password = matches[2]
			} else {
				re2 := regexp.MustCompile(`mysql://([^@]+)@`)
				matches2 := re2.FindStringSubmatch(dbInfo.URL)
				if len(matches2) >= 2 {
					user = matches2[1]
				}
			}
		}
		if password != "" {
			return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&timeout=5s", user, password, host, port, dbname)
		}
		return fmt.Sprintf("%s@tcp(%s:%s)/%s?parseTime=true&timeout=5s", user, host, port, dbname)
	}

	// PostgreSQL
	if dbInfo.URL != "" {
		dsn := dbInfo.URL
		if idx := strings.Index(dsn, "?"); idx != -1 {
			dsn = dsn[:idx]
		}
		return dsn
	}

	return fmt.Sprintf("postgres://%s:%s/%s?sslmode=disable", host, port, dbname)
}

// formatDuration форматирует секунды в HH:MM:SS
func formatDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

// formatSQLQuery форматирует SQL запрос для красивого вывода
func formatSQLQuery(query string) string {
	// Убираем лишние пробелы
	query = strings.TrimSpace(query)

	// Если запрос короткий, возвращаем как есть
	if len(query) <= 120 {
		return query
	}

	// Разбиваем на ключевые слова для форматирования
	keywords := []string{"SELECT", "FROM", "WHERE", "AND", "OR", "ORDER BY", "GROUP BY",
		"HAVING", "LIMIT", "OFFSET", "INSERT INTO", "VALUES", "UPDATE", "SET",
		"DELETE FROM", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "OUTER JOIN",
		"ON", "IN", "NOT IN", "EXISTS", "NOT EXISTS", "UNION", "ALL"}

	formatted := query
	for _, kw := range keywords {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
		formatted = re.ReplaceAllString(formatted, "\n  "+kw)
	}

	// Убираем лишний перенос в начале
	formatted = strings.TrimLeft(formatted, "\n ")

	// Если всё ещё длинный, обрезаем
	lines := strings.Split(formatted, "\n")
	if len(lines) > 10 {
		lines = lines[:10]
		lines = append(lines, "  ...")
	}

	return strings.Join(lines, "\n  ")
}
