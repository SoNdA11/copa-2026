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
			name: "add_avatar_url_to_users",
			sql:  `ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT DEFAULT ''`,
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
		{
			name: "add_home_team_label_to_matches",
			sql:  `ALTER TABLE matches ADD COLUMN IF NOT EXISTS home_team_label TEXT DEFAULT ''`,
		},
		{
			name: "add_away_team_label_to_matches",
			sql:  `ALTER TABLE matches ADD COLUMN IF NOT EXISTS away_team_label TEXT DEFAULT ''`,
		},
		{
			name: "add_advancing_team_id_to_bets",
			sql:  `ALTER TABLE bets ADD COLUMN IF NOT EXISTS advancing_team_id INTEGER DEFAULT 0`,
		},
		{
			name: "add_is_favorite_to_bets",
			sql:  `ALTER TABLE bets ADD COLUMN IF NOT EXISTS is_favorite INTEGER DEFAULT 0`,
		},
		{
			name: "create_user_stage_points",
			sql: `CREATE TABLE IF NOT EXISTS user_stage_points (
				user_id INTEGER NOT NULL,
				stage TEXT NOT NULL,
				common_points INTEGER DEFAULT 0,
				exact_score_bonus INTEGER DEFAULT 0,
				favorites_bonus INTEGER DEFAULT 0,
				total_points INTEGER DEFAULT 0,
				PRIMARY KEY (user_id, stage),
				FOREIGN KEY (user_id) REFERENCES users(id)
			)`,
		},
		{
			name: "backfill_knockout_labels_r32",
			sql: `UPDATE matches SET home_team_label = CASE id
				WHEN 73 THEN 'Runner-up Group A'
				WHEN 74 THEN 'Winner Group E'
				WHEN 75 THEN 'Winner Group F'
				WHEN 76 THEN 'Winner Group C'
				WHEN 77 THEN 'Winner Group I'
				WHEN 78 THEN 'Runner-up Group E'
				WHEN 79 THEN 'Winner Group A'
				WHEN 80 THEN 'Winner Group L'
				WHEN 81 THEN 'Winner Group D'
				WHEN 82 THEN 'Winner Group G'
				WHEN 83 THEN 'Runner-up Group K'
				WHEN 84 THEN 'Winner Group H'
				WHEN 85 THEN 'Winner Group B'
				WHEN 86 THEN 'Winner Group J'
				WHEN 87 THEN 'Winner Group K'
				WHEN 88 THEN 'Runner-up Group D'
				ELSE home_team_label END
			WHERE stage IN ('r32','r16','qf','sf','third','final') AND (home_team_label IS NULL OR home_team_label = '')`,
		},
		{
			name: "backfill_knockout_labels_r32_away",
			sql: `UPDATE matches SET away_team_label = CASE id
				WHEN 73 THEN 'Runner-up Group B'
				WHEN 74 THEN 'Runner-up Group D'
				WHEN 75 THEN 'Runner-up Group C'
				WHEN 76 THEN 'Runner-up Group F'
				WHEN 77 THEN '3rd Group F'
				WHEN 78 THEN '3rd Group I'
				WHEN 79 THEN '3rd Group E'
				WHEN 80 THEN '3rd Group E/H/I/J/K'
				WHEN 81 THEN '3rd Group B'
				WHEN 82 THEN '3rd Group A/E/H/I/J'
				WHEN 83 THEN 'Runner-up Group L'
				WHEN 84 THEN 'Runner-up Group J'
				WHEN 85 THEN '3rd Group E/F/G/I/J'
				WHEN 86 THEN 'Runner-up Group H'
				WHEN 87 THEN '3rd Group L'
				WHEN 88 THEN 'Runner-up Group G'
				ELSE away_team_label END
			WHERE stage IN ('r32','r16','qf','sf','third','final') AND (away_team_label IS NULL OR away_team_label = '')`,
		},
		{
			name: "backfill_knockout_labels_later",
			sql: `UPDATE matches SET home_team_label = CASE id
				WHEN 89 THEN 'Winner Match 74'   WHEN 90 THEN 'Winner Match 73'
				WHEN 91 THEN 'Winner Match 76'   WHEN 92 THEN 'Winner Match 79'
				WHEN 93 THEN 'Winner Match 83'   WHEN 94 THEN 'Winner Match 81'
				WHEN 95 THEN 'Winner Match 86'   WHEN 96 THEN 'Winner Match 85'
				WHEN 97 THEN 'Winner Match 89'   WHEN 98 THEN 'Winner Match 93'
				WHEN 99 THEN 'Winner Match 91'   WHEN 100 THEN 'Winner Match 95'
				WHEN 101 THEN 'Winner Match 97'  WHEN 102 THEN 'Winner Match 99'
				WHEN 104 THEN 'Winner Match 101'
				ELSE home_team_label END,
			away_team_label = CASE id
				WHEN 89 THEN 'Winner Match 77'   WHEN 90 THEN 'Winner Match 75'
				WHEN 91 THEN 'Winner Match 78'   WHEN 92 THEN 'Winner Match 80'
				WHEN 93 THEN 'Winner Match 84'   WHEN 94 THEN 'Winner Match 82'
				WHEN 95 THEN 'Winner Match 88'   WHEN 96 THEN 'Winner Match 87'
				WHEN 97 THEN 'Winner Match 90'   WHEN 98 THEN 'Winner Match 94'
				WHEN 99 THEN 'Winner Match 92'   WHEN 100 THEN 'Winner Match 96'
				WHEN 101 THEN 'Winner Match 98'  WHEN 102 THEN 'Winner Match 100'
				WHEN 104 THEN 'Winner Match 102'
				ELSE away_team_label END
			WHERE stage IN ('r16','qf','sf','final') AND ((home_team_label IS NULL OR home_team_label = '') OR (away_team_label IS NULL OR away_team_label = ''))`,
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
