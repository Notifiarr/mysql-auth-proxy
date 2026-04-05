package userinfo

import (
	"context"
	"fmt"
	"time"
)

const getUserQuery = "SELECT `developmentEnv`,`environment`,`name`,`id` FROM `users` WHERE `apikey`= ? " +
	"OR `id` = (SELECT `user_id` FROM `apikeys` WHERE `apikey`= ? LIMIT 1) LIMIT 1"

// GetInfo returns a user's info from a mysql database.
func (u *UI) GetInfo(ctx context.Context, requestKey string) (*UserInfo, error) {
	start := time.Now()

	rows, err := u.dbase.QueryContext(ctx, getUserQuery, requestKey, requestKey)
	u.metrics.QueryTime.WithLabelValues("users").Observe(time.Since(start).Seconds())

	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("users").Inc()
		return nil, fmt.Errorf("querying database: %w", err)
	}

	defer rows.Close() //nolint:errcheck

	user := DefaultUser()
	user.APIKey = requestKey

	if !rows.Next() {
		err = rows.Err()
		if err != nil {
			u.metrics.QueryErrors.WithLabelValues("users").Inc()
			return nil, fmt.Errorf("iterating database rows: %w", err)
		}

		u.metrics.QueryMissing.WithLabelValues("users").Inc()

		return user, ErrNoUser // must return default user on error.
	}

	devAllowed := "0"

	err = rows.Scan(&devAllowed, &user.Environment, &user.Username, &user.UserID)
	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("users").Inc()
		return nil, fmt.Errorf("scanning database rows: %w", err)
	}

	err = rows.Err()
	if err != nil { // we do not care at this point, scan on the first row worked fine...?
		u.metrics.QueryErrors.WithLabelValues("users").Inc()
		u.Printf("[ERROR] iterating database rows (ignored): %v", err)
	}

	if devAllowed != "1" {
		user.Environment = DefaultEnvironment
	}

	return user, nil
}
