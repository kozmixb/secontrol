package app

func (a *App) migrate() error {
	_, err := a.db.Exec(`
CREATE TABLE IF NOT EXISTS agents (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL, host TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 22,
 username TEXT NOT NULL, auth_type TEXT NOT NULL, credential BLOB NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', last_error TEXT NOT NULL DEFAULT '', last_seen DATETIME,
 os TEXT NOT NULL DEFAULT '', kernel TEXT NOT NULL DEFAULT '', uptime_seconds INTEGER NOT NULL DEFAULT 0,
 load1 REAL NOT NULL DEFAULT 0, memory_total INTEGER NOT NULL DEFAULT 0, memory_used INTEGER NOT NULL DEFAULT 0,
 disk_total INTEGER NOT NULL DEFAULT 0, disk_used INTEGER NOT NULL DEFAULT 0, cpu_count INTEGER NOT NULL DEFAULT 0,
 is_vm INTEGER NOT NULL DEFAULT 0, virtualization TEXT NOT NULL DEFAULT '', access_level TEXT NOT NULL DEFAULT 'regular',
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS ssh_keys (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, private_key BLOB NOT NULL, public_key TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS containers (
 agent_id INTEGER NOT NULL, id TEXT NOT NULL, name TEXT NOT NULL, image TEXT NOT NULL, status TEXT NOT NULL,
 state TEXT NOT NULL, ports TEXT NOT NULL, created TEXT NOT NULL, uptime TEXT NOT NULL DEFAULT '', updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 image_version TEXT NOT NULL DEFAULT '', update_available INTEGER, registry_digest TEXT NOT NULL DEFAULT '', image_checked_at DATETIME,
 available_version TEXT NOT NULL DEFAULT '',
 version_checked INTEGER NOT NULL DEFAULT 0,
 compose_project TEXT NOT NULL DEFAULT '', compose_service TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(agent_id,id), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS metric_samples (
 id INTEGER PRIMARY KEY, agent_id INTEGER NOT NULL, recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 load1 REAL NOT NULL, memory_used INTEGER NOT NULL, disk_used INTEGER NOT NULL,
 FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS network_interfaces (
 agent_id INTEGER NOT NULL, name TEXT NOT NULL, state TEXT NOT NULL DEFAULT '', mac TEXT NOT NULL DEFAULT '', addresses TEXT NOT NULL DEFAULT '[]',
 PRIMARY KEY(agent_id,name), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS system_services (
 agent_id INTEGER NOT NULL, name TEXT NOT NULL, load_state TEXT NOT NULL DEFAULT '', active_state TEXT NOT NULL DEFAULT '', sub_state TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(agent_id,name), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS storage_volumes (
 agent_id INTEGER NOT NULL, filesystem TEXT NOT NULL, fs_type TEXT NOT NULL DEFAULT '', mount_point TEXT NOT NULL,
 total_bytes INTEGER NOT NULL DEFAULT 0, used_bytes INTEGER NOT NULL DEFAULT 0, available_bytes INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(agent_id,filesystem,mount_point), FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_metrics_agent_time ON metric_samples(agent_id, recorded_at);
`)
	if err != nil {
		return err
	}

	// Ordered compatibility migrations for databases created by earlier releases.
	compatibilityMigrations := []string{
		"ALTER TABLE ssh_keys ADD COLUMN public_key TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE agents ADD COLUMN ssh_key_id INTEGER REFERENCES ssh_keys(id)",
		"ALTER TABLE agents ADD COLUMN is_vm INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE agents ADD COLUMN virtualization TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE agents ADD COLUMN access_level TEXT NOT NULL DEFAULT 'regular'",
		"ALTER TABLE containers ADD COLUMN uptime TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE containers ADD COLUMN image_version TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE containers ADD COLUMN update_available INTEGER",
		"ALTER TABLE containers ADD COLUMN registry_digest TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE containers ADD COLUMN image_checked_at DATETIME",
		"ALTER TABLE containers ADD COLUMN compose_project TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE containers ADD COLUMN compose_service TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE containers ADD COLUMN available_version TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE containers ADD COLUMN version_checked INTEGER NOT NULL DEFAULT 0",
	}
	for _, migration := range compatibilityMigrations {
		_, _ = a.db.Exec(migration)
	}
	return nil
}
