package userinfo

import (
	"context"
	"expvar"
	"fmt"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/prometheus/client_golang/prometheus"
)

const getUserQuery = "SELECT `developmentEnv`,`environment`,`name`,`id` FROM `users` WHERE `apikey`= ? " +
	"OR `id` = (SELECT `user_id` FROM `apikeys` WHERE `apikey`= ? LIMIT 1) LIMIT 1"

// GetInfo returns a user's info from a mysql database.
func (u *UI) GetInfo(ctx context.Context, requestKey string) (*UserInfo, error) {
	u.exp.Add("User Queries", 1)
	u.exp.Set("Last User", expvar.Func((&exp.Time{Time: time.Now()}).Since))

	timer := prometheus.NewTimer(u.metrics.QueryTime.WithLabelValues("users"))
	rows, err := u.dbase.QueryContext(ctx, getUserQuery, requestKey, requestKey)

	timer.ObserveDuration()

	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("users").Inc()
		u.exp.Add("User Errors", 1)

		return nil, fmt.Errorf("querying database: %w", err)
	} else if err = rows.Err(); err != nil {
		u.metrics.QueryErrors.WithLabelValues("users").Inc()
		u.exp.Add("User Errors", 1)

		return nil, fmt.Errorf("getting database rows: %w", err)
	}

	defer rows.Close()

	user := DefaultUser()
	user.APIKey = requestKey

	if !rows.Next() {
		u.metrics.QueryMissing.WithLabelValues("users").Inc()
		u.exp.Add("Missing Users", 1)

		return user, ErrNoUser // must return default user on error.
	}

	devAllowed := "0"

	err = rows.Scan(&devAllowed, &user.Environment, &user.Username, &user.UserID)
	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("users").Inc()
		u.exp.Add("User Errors", 1)

		return nil, fmt.Errorf("scanning database rows: %w", err)
	}

	if devAllowed != "1" {
		user.Environment = DefaultEnvironment
	}

	return user, nil
}
