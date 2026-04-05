// Package userinfo contains the methods to get Notifiarr user and server information from mysql.
package userinfo

import (
	"context"
	"fmt"
	"time"
)

const getServerQuery = "SELECT `apikey`,`developmentEnv`,`environment`,`name`,`id`,`discordServer` " +
	"FROM `users` WHERE `discordServer` = ?"

// GetServer retrieves a Discord server's information from the database.
func (u *UI) GetServer(ctx context.Context, serverID string) (*UserInfo, error) {
	start := time.Now()

	rows, err := u.dbase.QueryContext(ctx, getServerQuery, serverID)
	u.metrics.QueryTime.WithLabelValues("servers").Observe(time.Since(start).Seconds())

	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("servers").Inc()
		return nil, fmt.Errorf("querying database: %w", err)
	}

	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		user := DefaultUser()
		devAllowed := "0"
		discord := ""

		err := rows.Scan(&user.APIKey, &devAllowed, &user.Environment, &user.Username, &user.UserID, &discord)
		if err != nil {
			u.Printf("[ERROR] scanning mysql rows: %v", err)
			u.metrics.QueryErrors.WithLabelValues("servers").Inc()

			continue
		}

		if devAllowed != "1" {
			user.Environment = DefaultEnvironment
		}

		return user, nil
	}

	err = rows.Err()
	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("servers").Inc()
		return nil, fmt.Errorf("iterating database rows: %w", err)
	}

	u.metrics.QueryMissing.WithLabelValues("servers").Inc()

	return DefaultUser(), nil
}
