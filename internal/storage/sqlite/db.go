// Пакет sqlite — реализация репозиториев на SQLite.
//
// Используется драйвер modernc.org/sqlite: чистый Go, без системного
// C toolchain (docs/plan-mvp.md, TASK-080..086).
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB — обёртка над SQLite с автоматической инициализацией схемы.
type DB struct {
	*sql.DB
}

// Open открывает (или создаёт) базу и инициализирует схему (TASK-080).
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: не удалось открыть базу: %w", err)
	}

	db := &DB{DB: conn}
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

// initSchema создаёт таблицы, если их ещё нет.
func (db *DB) initSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id          TEXT PRIMARY KEY,
	title       TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	constraints TEXT NOT NULL DEFAULT '[]',
	mode        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS debates (
	id      TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE TABLE IF NOT EXISTS debate_rounds (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	debate_id     TEXT NOT NULL,
	round_number  INTEGER NOT NULL,
	proposal_a    TEXT NOT NULL DEFAULT '{}',
	proposal_b    TEXT NOT NULL DEFAULT '{}',
	critique_a    TEXT NOT NULL DEFAULT '{}',
	critique_b    TEXT NOT NULL DEFAULT '{}',
	revision_a    TEXT NOT NULL DEFAULT '{}',
	revision_b    TEXT NOT NULL DEFAULT '{}',
	UNIQUE (debate_id, round_number),
	FOREIGN KEY (debate_id) REFERENCES debates(id)
);

CREATE TABLE IF NOT EXISTS decisions (
	debate_id            TEXT PRIMARY KEY,
	status               TEXT NOT NULL,
	decision             TEXT NOT NULL DEFAULT '',
	confidence           REAL NOT NULL DEFAULT 0,
	supporting_arguments TEXT NOT NULL DEFAULT '[]',
	rejected_arguments   TEXT NOT NULL DEFAULT '[]',
	risks                TEXT NOT NULL DEFAULT '[]',
	unresolved_issues    TEXT NOT NULL DEFAULT '[]',
	FOREIGN KEY (debate_id) REFERENCES debates(id)
);

CREATE TABLE IF NOT EXISTS traces (
	trace_id    TEXT NOT NULL,
	task_id     TEXT NOT NULL,
	ts          INTEGER NOT NULL,
	event_type  TEXT NOT NULL,
	participant TEXT NOT NULL DEFAULT '',
	duration_ms INTEGER NOT NULL DEFAULT 0,
	metadata    TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS benchmark_runs (
	item_id    TEXT NOT NULL,
	task_id    TEXT PRIMARY KEY,
	mode       TEXT NOT NULL,
	models     TEXT NOT NULL DEFAULT '',
	rounds     INTEGER NOT NULL DEFAULT 0,
	latency_ms INTEGER NOT NULL DEFAULT 0,
	tokens     INTEGER NOT NULL DEFAULT 0,
	status     TEXT NOT NULL,
	decision   TEXT NOT NULL DEFAULT '',
	score      REAL NOT NULL DEFAULT -1
);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("sqlite: не удалось инициализировать схему: %w", err)
	}
	return nil
}

// Close закрывает соединение с базой.
func (db *DB) Close() error {
	return db.DB.Close()
}