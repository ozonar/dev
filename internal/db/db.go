package db

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"dev/internal/detector"
	"dev/internal/i18n"

	"github.com/fatih/color"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// ColumnInfo описывает колонку таблицы.
type ColumnInfo struct {
	Name     string
	Type     string
	Nullable string
	Key      string
}

// Run запускает интерактивный обзор базы данных.
func Run() error {
	cwd, _ := os.Getwd()
	info, err := detector.DetectProject(cwd)
	if err != nil {
		i18n.Red("Project analysis error: %v", err)
		return err
	}

	databases := info.Databases
	if len(databases) == 0 {
		i18n.Yellow("No databases found in the project.")
		i18n.Yellow("Check .env file or docker-compose configurations.")
		return nil
	}

	i18n.Cyan("=== Found databases ===")
	for i, db := range databases {
		locationColor := color.New(color.FgCyan).SprintFunc()
		switch db.Location {
		case detector.LocationLocal:
			locationColor = color.New(color.FgGreen).SprintFunc()
		case detector.LocationDocker:
			locationColor = color.New(color.FgYellow).SprintFunc()
		case detector.LocationRemote:
			locationColor = color.New(color.FgRed).SprintFunc()
		}
		fmt.Printf("%d. %s://%s:%s/%s (%s)\n",
			i+1,
			db.Type,
			db.Host,
			db.Port,
			db.Database,
			locationColor(db.Location),
		)
	}

	// Database selection (even if only one)
	fmt.Println()
	i18n.Printf("Select database (default 1): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	selectedIndex := 0
	if input == "" {
		selectedIndex = 0
	} else {
		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(databases) {
			i18n.Red("Invalid choice, using first database.")
			selectedIndex = 0
		} else {
			selectedIndex = idx - 1
		}
	}

	selectedDB := databases[selectedIndex]
	i18n.Green("Connecting to %s...", selectedDB.URL)

	// Connection
	db, err := connectDB(selectedDB)
	if err != nil {
		i18n.Red("Connection error: %v", err)
		return err
	}
	defer db.Close()

	i18n.Green("Connection successful.")

	// Table selection loop
	for {
		tables, err := listTables(db, selectedDB.Type)
		if err != nil {
			i18n.Red("Error fetching tables: %v", err)
			return err
		}

		if len(tables) == 0 {
			i18n.Yellow("No tables in selected database.")
			return nil
		}

		fmt.Println()
		i18n.Cyan("=== Tables in database ===")
		for i, tbl := range tables {
			count, err := getTableRowCount(db, selectedDB.Type, tbl)
			if err != nil || count < 0 {
				fmt.Printf("%d. %s\n", i+1, tbl)
			} else {
				fmt.Printf("%d. %s (%d rows)\n", i+1, tbl, count)
			}
		}

		fmt.Println()
		i18n.Printf("Select table (0 to exit): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "0" {
			i18n.Yellow("Exiting.")
			return nil
		}
		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(tables) {
			i18n.Red("Invalid choice.")
			continue
		}
		tableName := tables[idx-1]

		// Show table structure
		columns, err := describeTable(db, selectedDB.Type, tableName)
		if err != nil {
			i18n.Red("Error fetching table structure: %v", err)
			continue
		}

		// Get exact row count
		exactCount, err := getExactRowCount(db, selectedDB.Type, tableName)
		if err != nil {
			fmt.Println()
			i18n.Cyan("=== Table structure: %s ===", tableName)
		} else {
			i18n.Green("Total rows: %d", exactCount)
			fmt.Println()
			i18n.Cyan("=== Table structure: %s ===", tableName)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, i18n.T("Field\tType\tNULL\tKey"))
		for _, col := range columns {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", col.Name, col.Type, col.Nullable, col.Key)
		}
		w.Flush()

		// Ask to show recent values
		fmt.Println()
		i18n.Printf("Show latest values? (Y/n): ")
		input, _ = reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if strings.ToLower(input) == "n" || strings.ToLower(input) == "no" {
			i18n.Yellow("Skipping data.")
			continue
		}

		// Get last 20 rows
		rows, err := getLastRows(db, selectedDB.Type, tableName, 20)
		if err != nil {
			i18n.Red("Error fetching data: %v", err)
			continue
		}

		if len(rows) == 0 {
			i18n.Yellow("Table is empty.")
			continue
		}

		fmt.Println()
		i18n.Cyan("=== Last %d rows ===", len(rows))
		// Determine column widths for pretty output
		colWidths := make(map[string]int)
		for _, col := range columns {
			colWidths[col.Name] = len(col.Name)
		}
		for _, row := range rows {
			for _, col := range columns {
				val := row[col.Name]
				if val == nil {
					val = "NULL"
				}
				strVal := fmt.Sprintf("%v", val)
				if len(strVal) > colWidths[col.Name] {
					colWidths[col.Name] = len(strVal)
				}
			}
		}

		// Header
		header := ""
		for _, col := range columns {
			header += fmt.Sprintf("%-*s ", colWidths[col.Name], col.Name)
		}
		color.Yellow(header)

		// Divider
		divider := ""
		for _, col := range columns {
			divider += strings.Repeat("-", colWidths[col.Name]) + " "
		}
		fmt.Println(divider)

		// Data
		for _, row := range rows {
			line := ""
			for _, col := range columns {
				val := row[col.Name]
				strVal := "NULL"
				if val != nil {
					switch v := val.(type) {
					case []byte:
						// PostgreSQL возвращает UUID как []byte
						strVal = string(v)
					case [16]byte:
						// Некоторые драйверы могут вернуть фиксированный массив
						strVal = string(v[:])
					default:
						strVal = fmt.Sprintf("%v", v)
					}
				}
				line += fmt.Sprintf("%-*s ", colWidths[col.Name], strVal)
			}
			fmt.Println(line)
		}
		fmt.Println()
		i18n.Printf("Press Enter to continue...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
}

// connectDB connects to a database based on DatabaseInfo
func connectDB(dbInfo detector.DatabaseInfo) (*sql.DB, error) {
	var driver string
	var dsn string

	switch dbInfo.Type {
	case "postgresql":
		driver = "postgres"
		// Remove query parameters from URL as lib/pq doesn't support them
		dsn = dbInfo.URL
		if idx := strings.Index(dsn, "?"); idx != -1 {
			dsn = dsn[:idx]
		}
	case "mysql":
		driver = "mysql"
		// Convert URL to MySQL DSN (user:pass@tcp(host:port)/dbname)
		// Simplified: assume URL already in mysql://user:pass@host:port/dbname format
		dsn = strings.TrimPrefix(dbInfo.URL, "mysql://")
		// Add protocol if needed
		if !strings.Contains(dsn, "tcp(") {
			dsn = dsn + "?parseTime=true"
		}
	case "sqlite":
		driver = "sqlite3"
		dsn = dbInfo.Database // file path
	default:
		return nil, fmt.Errorf(i18n.T("unsupported DB type: %s"), dbInfo.Type)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}

// listTables возвращает список таблиц в базе данных.
func listTables(db *sql.DB, dbType string) ([]string, error) {
	var query string
	switch dbType {
	case "postgresql":
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name`
	case "mysql":
		query = `SHOW TABLES`
	case "sqlite":
		query = `SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`
	default:
		return nil, fmt.Errorf(i18n.T("unsupported DB type for table listing: %s"), dbType)
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		err := rows.Scan(&table)
		if err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}

// getTableRowCount возвращает оценочное число строк в таблице
// (быстрый способ, может вернуть -1).
func getTableRowCount(db *sql.DB, dbType, tableName string) (int, error) {
	var query string
	switch dbType {
	case "postgresql":
		query = `SELECT reltuples::bigint AS estimate FROM pg_class WHERE relname = $1`
	case "mysql":
		query = `SELECT TABLE_ROWS FROM information_schema.TABLES WHERE TABLE_NAME = ?`
	case "sqlite":
		query = `SELECT COUNT(*) FROM "` + tableName + `"`
	default:
		return 0, fmt.Errorf(i18n.T("unsupported DB type for row count: %s"), dbType)
	}

	var count int
	err := db.QueryRow(query, tableName).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// getExactRowCount возвращает точное число строк в таблице через COUNT(*).
func getExactRowCount(db *sql.DB, dbType, tableName string) (int, error) {
	var query string
	switch dbType {
	case "postgresql":
		query = `SELECT COUNT(*) FROM "` + tableName + `"`
	case "mysql":
		query = "SELECT COUNT(*) FROM `" + tableName + "`"
	case "sqlite":
		query = `SELECT COUNT(*) FROM "` + tableName + `"`
	default:
		return 0, fmt.Errorf(i18n.T("unsupported DB type for exact row count: %s"), dbType)
	}

	var count int
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// describeTable возвращает информацию о колонках таблицы.
func describeTable(db *sql.DB, dbType, tableName string) ([]ColumnInfo, error) {
	var query string
	switch dbType {
	case "postgresql":
		query = `
			SELECT column_name, data_type, is_nullable, column_default
			FROM information_schema.columns
			WHERE table_name = $1
			ORDER BY ordinal_position`
	case "mysql":
		query = `DESCRIBE ` + tableName
	case "sqlite":
		query = `PRAGMA table_info(` + tableName + `)`
	default:
		return nil, fmt.Errorf(i18n.T("unsupported DB type for table description: %s"), dbType)
	}

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		switch dbType {
		case "postgresql":
			var defaultValue *string
			err := rows.Scan(&col.Name, &col.Type, &col.Nullable, &defaultValue)
			if err != nil {
				return nil, err
			}
			col.Key = ""
		case "mysql":
			var null, key, extraStr string
			var defaultVal *string
			err := rows.Scan(&col.Name, &col.Type, &null, &key, &defaultVal, &extraStr)
			if err != nil {
				return nil, err
			}
			col.Nullable = null
			col.Key = key
		case "sqlite":
			var cid int
			var notnull int
			var dfltValue *string
			var pk int
			err := rows.Scan(&cid, &col.Name, &col.Type, &notnull, &dfltValue, &pk)
			if err != nil {
				return nil, err
			}
			if notnull == 0 {
				col.Nullable = "YES"
			} else {
				col.Nullable = "NO"
			}
			if pk == 1 {
				col.Key = "PRI"
			} else {
				col.Key = ""
			}
		}
		columns = append(columns, col)
	}
	return columns, nil
}

// getPrimaryKey возвращает имя колонки первичного ключа для таблицы PostgreSQL.
func getPrimaryKey(db *sql.DB, tableName string) (string, error) {
	query := `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
			AND tc.table_name = kcu.table_name
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_name = $1
		ORDER BY kcu.ordinal_position
		LIMIT 1`
	var pk string
	err := db.QueryRow(query, tableName).Scan(&pk)
	if err != nil {
		return "", err
	}
	return pk, nil
}

// getColumnNames возвращает список колонок таблицы для PostgreSQL,
// исключая системные колонки (ctid, oid, xmin, cmin, xmax, cmax, tableoid).
func getColumnNames(db *sql.DB, tableName string) ([]string, error) {
	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = $1
		ORDER BY ordinal_position`
	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// getLastRows возвращает последние N строк таблицы.
func getLastRows(db *sql.DB, dbType, tableName string, limit int) ([]map[string]any, error) {
	var query string
	var args []any

	switch dbType {
	case "postgresql":
		// Явно получаем список колонок через information_schema,
		// чтобы избежать системных колонок (ctid, oid и т.д.),
		// которые lib/pq может возвращать при SELECT *
		cols, err := getColumnNames(db, tableName)
		if err != nil {
			return nil, fmt.Errorf(i18n.T("error getting column names: %w"), err)
		}
		if len(cols) == 0 {
			return nil, fmt.Errorf(i18n.T("no columns found for table %s"), tableName)
		}

		// Экранируем имена колонок кавычками
		quotedCols := make([]string, len(cols))
		for i, c := range cols {
			quotedCols[i] = `"` + c + `"`
		}
		colList := strings.Join(quotedCols, ", ")

		pk, err := getPrimaryKey(db, tableName)
		if err != nil || pk == "" {
			query = fmt.Sprintf(`SELECT %s FROM "%s" LIMIT $1`, colList, tableName)
			args = []any{limit}
		} else {
			query = fmt.Sprintf(`SELECT %s FROM "%s" ORDER BY "%s" DESC LIMIT $1`, colList, tableName, pk)
			args = []any{limit}
		}
	case "mysql", "sqlite":
		query = `SELECT * FROM ` + tableName + ` LIMIT ` + strconv.Itoa(limit)
		args = []any{}
	default:
		return nil, fmt.Errorf(i18n.T("unsupported DB type for data selection: %s"), dbType)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		err := rows.Scan(valuePtrs...)
		if err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range cols {
			val := values[i]
			row[col] = val
		}
		result = append(result, row)
	}
	return result, nil
}
