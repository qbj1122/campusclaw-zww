package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, dsn string, log *slog.Logger) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	var last error
	for i := 0; i < 40; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		last = db.PingContext(pingCtx)
		cancel()
		if last == nil {
			log.Info("database connected")
			return db, nil
		}
		log.Warn("database not ready", "attempt", i+1, "error", last.Error())
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("database unreachable: %w", last)
}

// Migrate creates every table with IF NOT EXISTS so repeated starts are safe.
func Migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	for _, stmt := range schema {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w\nstatement: %s", err, stmt)
		}
	}
	// ngram parser is MySQL 8 specific. If the local server lacks it we still
	// create a plain FULLTEXT index; keyword search also has a LIKE fallback.
	if _, err := db.ExecContext(ctx, `CREATE FULLTEXT INDEX ft_chunk_text ON knowledge_chunks(chunk_text) WITH PARSER ngram`); err != nil {
		if !isDuplicateIndex(err) {
			log.Warn("ngram fulltext index unavailable, falling back", "error", err.Error())
			if _, err2 := db.ExecContext(ctx, `CREATE FULLTEXT INDEX ft_chunk_text ON knowledge_chunks(chunk_text)`); err2 != nil && !isDuplicateIndex(err2) {
				log.Warn("plain fulltext index unavailable; LIKE fallback will be used", "error", err2.Error())
			}
		}
	}
	return nil
}

func isDuplicateIndex(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "duplicate key name") || strings.Contains(s, "already exists") || strings.Contains(s, "1061")
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS classes (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(64) NOT NULL UNIQUE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS users (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(64) NOT NULL UNIQUE,
		display_name VARCHAR(64) NOT NULL DEFAULT '',
		role ENUM('teacher','student') NOT NULL,
		class_id BIGINT NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_users_class (class_id),
		CONSTRAINT fk_users_class FOREIGN KEY (class_id) REFERENCES classes(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS token_revocations (
		jti VARCHAR(64) PRIMARY KEY,
		expires_at DATETIME NOT NULL,
		KEY idx_revocations_exp (expires_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS class_settings (
		class_id BIGINT PRIMARY KEY,
		prompt TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		CONSTRAINT fk_settings_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS materials (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		uploaded_by BIGINT NOT NULL,
		title VARCHAR(255) NOT NULL,
		file_path VARCHAR(512) NOT NULL,
		mime_type VARCHAR(128) NOT NULL DEFAULT 'text/plain',
		size_bytes BIGINT NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_materials_class (class_id),
		CONSTRAINT fk_materials_class FOREIGN KEY (class_id) REFERENCES classes(id),
		CONSTRAINT fk_materials_user FOREIGN KEY (uploaded_by) REFERENCES users(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS knowledge_entries (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		material_id BIGINT NOT NULL UNIQUE,
		class_id BIGINT NOT NULL,
		body_text MEDIUMTEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_entries_class (class_id),
		CONSTRAINT fk_entries_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE,
		CONSTRAINT fk_entries_class FOREIGN KEY (class_id) REFERENCES classes(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS knowledge_chunks (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		material_id BIGINT NOT NULL,
		knowledge_entry_id BIGINT NOT NULL,
		chunk_index INT NOT NULL,
		chunk_text MEDIUMTEXT NOT NULL,
		start_offset INT NOT NULL,
		end_offset INT NOT NULL,
		index_status ENUM('pending','ready','failed') NOT NULL DEFAULT 'pending',
		chunk_strategy VARCHAR(16) NOT NULL DEFAULT 'auto',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uk_entry_chunk (knowledge_entry_id, chunk_index),
		KEY idx_chunks_class (class_id),
		KEY idx_chunks_material (material_id),
		KEY idx_chunks_status (class_id, index_status),
		CONSTRAINT fk_chunks_entry FOREIGN KEY (knowledge_entry_id) REFERENCES knowledge_entries(id) ON DELETE CASCADE,
		CONSTRAINT fk_chunks_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE,
		CONSTRAINT fk_chunks_class FOREIGN KEY (class_id) REFERENCES classes(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS knowledge_index_state (
		knowledge_entry_id BIGINT PRIMARY KEY,
		body_hash CHAR(64) NOT NULL,
		strategy VARCHAR(16) NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		CONSTRAINT fk_index_state_entry FOREIGN KEY (knowledge_entry_id) REFERENCES knowledge_entries(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS skills (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		name VARCHAR(32) NOT NULL,
		trigger_text VARCHAR(120) NOT NULL,
		description TEXT NOT NULL,
		tools_json VARCHAR(1000) NOT NULL DEFAULT '[]',
		source_text MEDIUMTEXT NOT NULL,
		subject_text VARCHAR(32) NOT NULL DEFAULT '',
		enabled TINYINT(1) NOT NULL DEFAULT 1,
		seed_key VARCHAR(64) NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_skills_class_name (class_id, name),
		UNIQUE KEY uk_skills_seed (seed_key),
		KEY idx_skills_class (class_id),
		CONSTRAINT fk_skills_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS agents (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		name VARCHAR(64) NOT NULL,
		prompt TEXT NOT NULL,
		enabled TINYINT(1) NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_agents_class_name (class_id, name),
		KEY idx_agents_class (class_id),
		CONSTRAINT fk_agents_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS agent_skills (
		agent_id BIGINT NOT NULL,
		skill_id BIGINT NOT NULL,
		PRIMARY KEY (agent_id, skill_id),
		CONSTRAINT fk_as_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
		CONSTRAINT fk_as_skill FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS agent_materials (
		agent_id BIGINT NOT NULL,
		material_id BIGINT NOT NULL,
		PRIMARY KEY (agent_id, material_id),
		CONSTRAINT fk_am_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
		CONSTRAINT fk_am_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS conversations (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		user_id BIGINT NOT NULL,
		agent_id BIGINT NOT NULL,
		title VARCHAR(64) NOT NULL DEFAULT '新的提问',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		KEY idx_conversations_user (user_id, updated_at),
		KEY idx_conversations_class (class_id),
		CONSTRAINT fk_conv_class FOREIGN KEY (class_id) REFERENCES classes(id),
		CONSTRAINT fk_conv_user FOREIGN KEY (user_id) REFERENCES users(id),
		CONSTRAINT fk_conv_agent FOREIGN KEY (agent_id) REFERENCES agents(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		conversation_id BIGINT NOT NULL,
		role ENUM('user','assistant') NOT NULL,
		content MEDIUMTEXT NOT NULL,
		citations_json MEDIUMTEXT NULL,
		skill_name VARCHAR(64) NOT NULL DEFAULT '',
		tool_calls_json MEDIUMTEXT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_messages_conv (conversation_id, id),
		CONSTRAINT fk_msg_conv FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS mcp_servers (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		name VARCHAR(64) NOT NULL,
		base_url VARCHAR(512) NOT NULL,
		auth_token VARCHAR(512) NOT NULL DEFAULT '',
		connected TINYINT(1) NOT NULL DEFAULT 0,
		connected_at DATETIME NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_mcp_class_name (class_id, name),
		KEY idx_mcp_class (class_id),
		CONSTRAINT fk_mcp_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS mcp_tools (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		server_id BIGINT NOT NULL,
		tool_name VARCHAR(64) NOT NULL,
		description TEXT NOT NULL,
		schema_json MEDIUMTEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uk_mcp_tool (server_id, tool_name),
		CONSTRAINT fk_tool_server FOREIGN KEY (server_id) REFERENCES mcp_servers(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS mcp_tool_calls (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		tool_ref VARCHAR(128) NOT NULL,
		server_id BIGINT NOT NULL,
		status ENUM('ok','denied','error') NOT NULL,
		detail VARCHAR(512) NOT NULL DEFAULT '',
		user_id BIGINT NOT NULL,
		conversation_id BIGINT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_calls_class (class_id, created_at),
		KEY idx_calls_conv (conversation_id),
		CONSTRAINT fk_call_class FOREIGN KEY (class_id) REFERENCES classes(id),
		CONSTRAINT fk_call_server FOREIGN KEY (server_id) REFERENCES mcp_servers(id) ON DELETE RESTRICT,
		CONSTRAINT fk_call_user FOREIGN KEY (user_id) REFERENCES users(id),
		CONSTRAINT fk_call_conv FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE SET NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	// Lesson 2 required six core structures. handouts/assignments are
	// placeholders for the later course iterations, not used by the main path.
	`CREATE TABLE IF NOT EXISTS handouts (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		title VARCHAR(255) NOT NULL,
		body MEDIUMTEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_handouts_class (class_id),
		CONSTRAINT fk_handouts_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS assignments (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		class_id BIGINT NOT NULL,
		title VARCHAR(255) NOT NULL,
		description TEXT NOT NULL,
		due_at DATETIME NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_assignments_class (class_id),
		CONSTRAINT fk_assignments_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
}
