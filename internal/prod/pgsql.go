package prod

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// discoverPostgresDSN находит строку подключения к PostgreSQL из окружения
// или возвращает пустую строку, если БД не обнаружена.
func discoverPostgresDSN() string {
	for _, key := range []string{"DATABASE_URL", "POSTGRES_DSN", "DATABASE_DSN", "PG_DSN"} {
		if v := os.Getenv(key); v != "" && containsPostgres(v) {
			return v
		}
	}
	// Проверяем наличие локального postgres на стандартном порту.
	if isPortOpen("127.0.0.1:5432") {
		return "host=127.0.0.1 port=5432 sslmode=disable user=postgres"
	}
	return ""
}

func containsPostgres(dsn string) bool {
	return strings.Contains(dsn, "postgres") ||
		strings.Contains(dsn, "postgresql") ||
		strings.Contains(dsn, "pgsql")
}

// isPortOpen проверяет доступность TCP-порта с коротким таймаутом.
func isPortOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// collectPostgres собирает категорию PostgreSQL.
func collectPostgres(prev *Report) *Category {
	cat := &Category{ID: CatPostgres, Present: true}
	dsn := discoverPostgresDSN()
	if dsn == "" {
		// PostgreSQL не найден — категория не показывается.
		cat.Present = false
		return cat
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		cat.Present = false
		return cat
	}
	defer db.Close()
	db.SetConnMaxLifetime(5 * time.Second)
	if err := db.Ping(); err != nil {
		// Не удалось подключиться — не показываем категорию.
		cat.Present = false
		return cat
	}
	cat.Detected = true

	// DATABASE_LOCK_CONTENTION.
	if n, ok := pgInt(db, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock'`); ok {
		switch {
		case n >= 20:
			cat.AddSymptom(Symptom{ID: "DATABASE_LOCK_CONTENTION", Level: LevelError,
				Summary: fmt.Sprintf("%d queries waiting for locks", n)})
		case n > 0:
			cat.AddSymptom(Symptom{ID: "DATABASE_LOCK_CONTENTION", Level: LevelWarn,
				Summary: fmt.Sprintf("%d queries waiting for locks", n)})
		default:
			cat.AddSymptom(Symptom{ID: "DATABASE_LOCK_CONTENTION", Level: LevelOK,
				Summary: "no queries waiting for locks"})
		}
	}

	// DATABASE_LONG_TRANSACTION: самый старый активный транзакционный возраст.
	if secs, ok := pgFloat(db, `SELECT COALESCE(EXTRACT(EPOCH FROM (now() - xact_start)), 0)
		FROM pg_stat_activity WHERE xact_start IS NOT NULL AND state <> 'idle'
		ORDER BY xact_start ASC LIMIT 1`); ok && secs > 60 {
		lvl := LevelWarn
		if secs > 600 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "DATABASE_LONG_TRANSACTION", Level: lvl,
			Summary: fmt.Sprintf("longest transaction %s", durHuman(secs))})
	}

	// DATABASE_CONNECTION_EXHAUSTION.
	used, _ := pgInt(db, `SELECT count(*) FROM pg_stat_activity`)
	maxC, _ := pgInt(db, `SELECT current_setting('max_connections')::int`)
	if maxC > 0 && used > 0 {
		pct := float64(used) / float64(maxC) * 100
		switch {
		case pct >= 90:
			cat.AddSymptom(Symptom{ID: "DATABASE_CONNECTION_EXHAUSTION", Level: LevelError,
				Summary: fmt.Sprintf("%d/%d connections (%.0f%%)", used, maxC, pct)})
		case pct >= 70:
			cat.AddSymptom(Symptom{ID: "DATABASE_CONNECTION_EXHAUSTION", Level: LevelWarn,
				Summary: fmt.Sprintf("%d/%d connections (%.0f%%)", used, maxC, pct)})
		default:
			cat.AddSymptom(Symptom{ID: "DATABASE_CONNECTION_EXHAUSTION", Level: LevelOK,
				Summary: fmt.Sprintf("%d/%d connections", used, maxC)})
		}
	}

	// DATABASE_SLOW_QUERIES.
	if n, ok := pgInt(db, `SELECT count(*) FROM pg_stat_activity WHERE state = 'active'
		AND now() - query_start > interval '5 seconds'`); ok && n > 0 {
		cat.AddSymptom(Symptom{ID: "DATABASE_SLOW_QUERIES", Level: LevelWarn,
			Summary: fmt.Sprintf("%d active queries running > 5s", n)})
	}

	// DATABASE_REPLICATION_LAG.
	if lagSecs, ok := pgFloat(db, `SELECT COALESCE(MAX(EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))), 0)
		WHERE pg_is_in_recovery()`); ok && lagSecs > 30 {
		lvl := LevelWarn
		if lagSecs > 300 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "DATABASE_REPLICATION_LAG", Level: lvl,
			Summary: fmt.Sprintf("replication lag %s", durHuman(lagSecs))})
	}

	// DATABASE_CONNECTION_ERRORS: из журнала postgres.
	if n, ok := pgConnectionErrors(); ok && n > 0 {
		cat.AddSymptom(Symptom{ID: "DATABASE_CONNECTION_ERRORS", Level: LevelWarn,
			Summary: fmt.Sprintf("%d connection errors in recent log", n)})
	}

	cat.Data = "pgsql connected"
	return cat
}

// pgInt выполняет запрос, возвращающий одно целое число.
func pgInt(db *sql.DB, q string) (int64, bool) {
	var v int64
	if err := db.QueryRow(q).Scan(&v); err != nil {
		return 0, false
	}
	return v, true
}

// pgFloat выполняет запрос, возвращающий одно дробное число.
func pgFloat(db *sql.DB, q string) (float64, bool) {
	var v float64
	if err := db.QueryRow(q).Scan(&v); err != nil {
		return 0, false
	}
	return v, true
}

// pgConnectionErrors ищет ошибки соединений в логах postgres.
func pgConnectionErrors() (int, bool) {
	count := 0
	for _, unit := range []string{"postgresql", "postgresql.service"} {
		for _, l := range tailJournal(unit, 80) {
			low := strings.ToLower(l)
			if strings.Contains(low, "could not accept") || strings.Contains(low, "terminating connection") {
				count++
			}
		}
	}
	return count, count > 0
}

// durHuman форматирует секунды в человекочитаемый вид.
func durHuman(secs float64) string {
	if secs >= 3600 {
		return fmt.Sprintf("%.1fh", secs/3600)
	}
	if secs >= 60 {
		return fmt.Sprintf("%.1fm", secs/60)
	}
	return fmt.Sprintf("%.0fs", secs)
}
