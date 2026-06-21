package database

import (
	"database/sql"
	"fmt"
	"log"
)

func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "create_users",
			sql: `CREATE TABLE IF NOT EXISTS users (
				id SERIAL PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{
			name: "create_teams",
			sql: `CREATE TABLE IF NOT EXISTS teams (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				fifa_code TEXT,
				group_name TEXT NOT NULL,
				flag_url TEXT
			)`,
		},
		{
			name: "create_matches",
			sql: `CREATE TABLE IF NOT EXISTS matches (
				id INTEGER PRIMARY KEY,
				home_team_id INTEGER NOT NULL,
				away_team_id INTEGER NOT NULL,
				home_score INTEGER,
				away_score INTEGER,
				match_date TEXT NOT NULL,
				match_time TEXT NOT NULL,
				stage TEXT NOT NULL,
				group_name TEXT,
				stadium TEXT,
				status TEXT DEFAULT 'upcoming',
				FOREIGN KEY (home_team_id) REFERENCES teams(id),
				FOREIGN KEY (away_team_id) REFERENCES teams(id)
			)`,
		},
		{
			name: "create_bets",
			sql: `CREATE TABLE IF NOT EXISTS bets (
				id SERIAL PRIMARY KEY,
				user_id INTEGER NOT NULL,
				match_id INTEGER NOT NULL,
				home_score INTEGER NOT NULL,
				away_score INTEGER NOT NULL,
				points INTEGER DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id),
				FOREIGN KEY (match_id) REFERENCES matches(id),
				UNIQUE(user_id, match_id)
			)`,
		},
		{
			name: "add_admin_to_users",
			sql:  `ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin INTEGER DEFAULT 0`,
		},
		{
			name: "add_points_adjustment",
			sql:  `ALTER TABLE users ADD COLUMN IF NOT EXISTS points_adjustment INTEGER DEFAULT 0`,
		},
		{
			name: "create_special_bets",
			sql: `CREATE TABLE IF NOT EXISTS special_bets (
				id SERIAL PRIMARY KEY,
				user_id INTEGER NOT NULL,
				bet_type TEXT NOT NULL,
				value TEXT NOT NULL,
				points INTEGER DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id),
				UNIQUE(user_id, bet_type)
			)`,
		},
		{
			name: "create_groups",
			sql: `CREATE TABLE IF NOT EXISTS groups (
				id SERIAL PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				slug TEXT NOT NULL UNIQUE
			);
			INSERT INTO groups (name, slug) VALUES ('Taberna', 'taberna') ON CONFLICT DO NOTHING;
			INSERT INTO groups (name, slug) VALUES ('UERN', 'uern') ON CONFLICT DO NOTHING`,
		},
		{
			name: "add_group_id_to_users",
			sql: `ALTER TABLE users ADD COLUMN IF NOT EXISTS group_id INTEGER REFERENCES groups(id);
			UPDATE users SET group_id = 1 WHERE group_id IS NULL;
			ALTER TABLE users ALTER COLUMN group_id SET NOT NULL;
			ALTER TABLE users DROP CONSTRAINT IF EXISTS users_name_key;
			ALTER TABLE users ADD UNIQUE(name, group_id)`,
		},
		{
			name: "create_bet_history",
			sql: `CREATE TABLE IF NOT EXISTS bet_history (
				id SERIAL PRIMARY KEY,
				bet_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				match_id INTEGER NOT NULL,
				old_home_score INTEGER,
				old_away_score INTEGER,
				new_home_score INTEGER NOT NULL,
				new_away_score INTEGER NOT NULL,
				changed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (bet_id) REFERENCES bets(id),
				FOREIGN KEY (user_id) REFERENCES users(id),
				FOREIGN KEY (match_id) REFERENCES matches(id)
			)`,
		},
	}

	for _, m := range migrations {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name = $1", m.name).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", m.name, err)
		}
		if count > 0 {
			continue
		}

		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", m.name, err)
		}

		if _, err := db.Exec("INSERT INTO schema_migrations (name) VALUES ($1)", m.name); err != nil {
			return fmt.Errorf("failed to record migration %s: %w", m.name, err)
		}

		log.Printf("Migration applied: %s", m.name)
	}

	return nil
}
