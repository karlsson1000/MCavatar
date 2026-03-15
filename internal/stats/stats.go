package stats

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

var (
	db    *sql.DB
	reqCh = make(chan struct{}, 1024)
)

func Init(path string) {
	var err error
	db, err = sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Printf("open stats db: %v — stats disabled", err)
		return
	}

	db.SetMaxOpenConns(1)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS stats (
		key   TEXT PRIMARY KEY,
		value INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		log.Printf("create stats table: %v — stats disabled", err)
		db = nil
		return
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO stats (key, value) VALUES ('total_requests', 0)`)
	if err != nil {
		log.Printf("init stats row: %v — stats disabled", err)
		db = nil
		return
	}

	go func() {
		for range reqCh {
			if db != nil {
				db.Exec(`UPDATE stats SET value = value + 1 WHERE key = 'total_requests'`)
			}
		}
	}()
}

func IncrementRequests() {
	select {
	case reqCh <- struct{}{}:
	default:
	}
}

func GetTotalRequests() int64 {
	if db == nil {
		return 0
	}
	var n int64
	if err := db.QueryRow(`SELECT value FROM stats WHERE key = 'total_requests'`).Scan(&n); err != nil {
		log.Printf("stats read failed: %v", err)
		return 0
	}
	return n
}
