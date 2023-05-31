package userinfo

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/prometheus/client_golang/prometheus"
)

const getServerQuery = "SELECT `apikey`,`developmentEnv`,`environment`,`name`,`id`,`discordServer` " +
	"FROM `users` WHERE `discordServer` = ?"

func (u *UI) GetServer(ctx context.Context, serverID string) (*UserInfo, error) {
	u.exp.Add("Server Queries", 1)
	u.exp.Set("Last Server", expvar.Func((&exp.Time{Time: time.Now()}).Since))

	timer := prometheus.NewTimer(u.metrics.QueryTime.WithLabelValues("servers"))
	rows, err := u.dbase.QueryContext(ctx, getServerQuery, serverID)
	timer.ObserveDuration()

	if err != nil {
		u.exp.Add("Server Errors", 1)
		u.metrics.QueryErrors.WithLabelValues("servers").Inc()

		return nil, fmt.Errorf("querying database: %w", err)
	} else if err = rows.Err(); err != nil {
		u.exp.Add("Server Errors", 1)
		u.metrics.QueryErrors.WithLabelValues("servers").Inc()

		return nil, fmt.Errorf("getting database rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		user := DefaultUser()
		devAllowed := "0"
		discord := ""

		err := rows.Scan(&user.APIKey, &devAllowed, &user.Environment, &user.Username, &user.UserID, &discord)
		if err != nil {
			u.exp.Add("Server Errors", 1)
			u.Printf("[ERROR] scanning mysql rows: %v", err)
			u.metrics.QueryErrors.WithLabelValues("servers").Inc()

			continue
		}

		if devAllowed != "1" {
			user.Environment = DefaultEnvironment
		}

		discordVal := struct {
			Server string `json:"discordServer"`
		}{}

		if err = json.Unmarshal([]byte(discord), &discordVal); err != nil {
			u.exp.Add("Server Errors", 1)
			u.metrics.QueryErrors.WithLabelValues("servers").Inc()
			u.Printf("[ERROR] mysql json parse: %v", err)
		}

		if discordVal.Server == serverID {
			return user, nil
		}
	}

	u.metrics.QueryMissing.WithLabelValues("servers").Inc()
	u.exp.Add("Missing Servers", 1)

	return DefaultUser(), nil
}
