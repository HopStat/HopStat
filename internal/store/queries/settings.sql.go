package queries

import "database/sql"

func (q *Queries) GetSettings() (map[string]string, error) {
	rows, err := q.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		m[key] = value
	}
	return m, rows.Err()
}

func (q *Queries) GetSetting(key string) (string, error) {
	var value string
	err := q.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (q *Queries) SetSetting(key, value string) error {
	_, err := q.db.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?", key, value, value)
	return err
}

func (q *Queries) SetSettings(m map[string]string) error {
	for k, v := range m {
		if _, err := q.db.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?", k, v, v); err != nil {
			return err
		}
	}
	return nil
}
