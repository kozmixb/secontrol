package app

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

type StorageVolume struct {
	AgentID        int64  `json:"agentId"`
	AgentName      string `json:"agentName"`
	AgentHost      string `json:"agentHost"`
	AgentStatus    string `json:"agentStatus"`
	Filesystem     string `json:"filesystem"`
	Type           string `json:"type"`
	MountPoint     string `json:"mountPoint"`
	TotalBytes     int64  `json:"totalBytes"`
	UsedBytes      int64  `json:"usedBytes"`
	AvailableBytes int64  `json:"availableBytes"`
	IsRemote       bool   `json:"isRemote"`
}

func (a *App) listFleetStorage(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT v.agent_id,a.name,a.host,a.status,v.filesystem,v.fs_type,v.mount_point,v.total_bytes,v.used_bytes,v.available_bytes
		FROM storage_volumes v JOIN agents a ON a.id=v.agent_id
		WHERE lower(v.fs_type) <> 'overlay'
		ORDER BY a.name, CASE v.mount_point WHEN '/' THEN 0 ELSE 1 END, v.mount_point`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	volumes := []StorageVolume{}
	for rows.Next() {
		var volume StorageVolume
		if rows.Scan(&volume.AgentID, &volume.AgentName, &volume.AgentHost, &volume.AgentStatus, &volume.Filesystem, &volume.Type, &volume.MountPoint, &volume.TotalBytes, &volume.UsedBytes, &volume.AvailableBytes) == nil {
			volume.IsRemote = isRemoteFilesystem(volume.Type)
			volumes = append(volumes, volume)
		}
	}
	writeJSON(w, http.StatusOK, volumes)
}

func isRemoteFilesystem(fsType string) bool {
	switch strings.ToLower(strings.TrimSpace(fsType)) {
	case "nfs", "nfs4", "cifs", "smb3":
		return true
	default:
		return false
	}
}

func storeStorageInventory(ctx context.Context, tx *sql.Tx, agentID int64, section string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM storage_volumes WHERE agent_id=?", agentID); err != nil {
		return err
	}
	for _, line := range strings.Split(section, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 6)
		if len(parts) != 6 || parts[0] == "" || parts[5] == "" {
			continue
		}
		if strings.EqualFold(parts[1], "overlay") {
			continue
		}
		total, errTotal := strconv.ParseInt(parts[2], 10, 64)
		used, errUsed := strconv.ParseInt(parts[3], 10, 64)
		available, errAvailable := strconv.ParseInt(parts[4], 10, 64)
		if errTotal != nil || errUsed != nil || errAvailable != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO storage_volumes(agent_id,filesystem,fs_type,mount_point,total_bytes,used_bytes,available_bytes)
			VALUES(?,?,?,?,?,?,?)`, agentID, parts[0], parts[1], parts[5], total, used, available); err != nil {
			return err
		}
	}
	return nil
}
