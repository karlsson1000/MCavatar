package stats

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func Init(path string) {
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		log.Fatalf("open stats db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS stats (
		key   TEXT PRIMARY KEY,
		value INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		log.Fatalf("create stats table: %v", err)
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO stats (key, value) VALUES ('total_requests', 0)`)
	if err != nil {
		log.Fatalf("init stats row: %v", err)
	}
}

func IncrementRequests() {
	db.Exec(`UPDATE stats SET value = value + 1 WHERE key = 'total_requests'`)
}

func GetTotalRequests() int64 {
	var n int64
	db.QueryRow(`SELECT value FROM stats WHERE key = 'total_requests'`).Scan(&n)
	return n
}
