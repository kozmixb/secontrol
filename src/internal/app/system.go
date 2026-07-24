package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

type NetworkInterface struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	MAC       string   `json:"mac,omitempty"`
	Addresses []string `json:"addresses"`
}

type SystemService struct {
	Name        string `json:"name"`
	LoadState   string `json:"loadState"`
	ActiveState string `json:"activeState"`
	SubState    string `json:"subState"`
	Description string `json:"description,omitempty"`
}

func (a *App) machineSystem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	networks := []NetworkInterface{}
	rows, err := a.db.QueryContext(r.Context(), "SELECT name,state,mac,addresses FROM network_interfaces WHERE agent_id=? ORDER BY name", id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var item NetworkInterface
		var addresses string
		if rows.Scan(&item.Name, &item.State, &item.MAC, &addresses) == nil {
			_ = json.Unmarshal([]byte(addresses), &item.Addresses)
			networks = append(networks, item)
		}
	}
	rows.Close()

	services := []SystemService{}
	rows, err = a.db.QueryContext(r.Context(), "SELECT name,load_state,active_state,sub_state,description FROM system_services WHERE agent_id=? ORDER BY CASE active_state WHEN 'failed' THEN 0 WHEN 'active' THEN 1 ELSE 2 END,name", id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item SystemService
		if rows.Scan(&item.Name, &item.LoadState, &item.ActiveState, &item.SubState, &item.Description) == nil {
			services = append(services, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"networks": networks, "services": services})
}

func storeSystemInventory(ctx context.Context, tx *sql.Tx, agentID int64, sections map[string]string) error {
	if err := storeStorageInventory(ctx, tx, agentID, sections["STORAGE"]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM network_interfaces WHERE agent_id=?", agentID); err != nil {
		return err
	}
	var rawNetworks []struct {
		Name      string `json:"ifname"`
		State     string `json:"operstate"`
		MAC       string `json:"address"`
		Addresses []struct {
			Family    string `json:"family"`
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
			Scope     string `json:"scope"`
		} `json:"addr_info"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(sections["INTERFACES"])), &rawNetworks) == nil {
		for _, raw := range rawNetworks {
			addresses := []string{}
			for _, address := range raw.Addresses {
				if address.Local != "" {
					addresses = append(addresses, address.Local+"/"+itoa(address.PrefixLen))
				}
			}
			encoded, _ := json.Marshal(addresses)
			if _, err := tx.ExecContext(ctx, "INSERT INTO network_interfaces(agent_id,name,state,mac,addresses) VALUES(?,?,?,?,?)", agentID, raw.Name, strings.ToLower(raw.State), raw.MAC, string(encoded)); err != nil {
				return err
			}
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM system_services WHERE agent_id=?", agentID); err != nil {
		return err
	}
	for _, line := range strings.Split(sections["SERVICES"], "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 5)
		if len(parts) != 5 || parts[0] == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO system_services(agent_id,name,load_state,active_state,sub_state,description) VALUES(?,?,?,?,?,?)", agentID, parts[0], parts[1], parts[2], parts[3], parts[4]); err != nil {
			return err
		}
	}
	return nil
}

func itoa(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(digits[value%10]) + result
		value /= 10
	}
	return result
}
