package userinfo

import (
	"context"
	"fmt"
	"time"
)

// access_expires is a unix timestamp written by the website. A zero value is a
// PKCE challenge stored in access_token, not a usable link token.
const getAlexaUserQuery = "SELECT u.`apikey`, u.`developmentEnv`, u.`environment`, u.`name`, u.`id` " +
	"FROM `alexa_oauth` a JOIN `users` u ON u.`id` = a.`user_id` " +
	"WHERE a.`access_token` = ? AND a.`access_expires` > UNIX_TIMESTAMP() LIMIT 1"

// GetAlexa returns the user linked to an Alexa access token.
// The token is session.user.accessToken from an Alexa skill request.
func (u *UI) GetAlexa(ctx context.Context, accessToken string) (*UserInfo, error) {
	start := time.Now()

	rows, err := u.dbase.QueryContext(ctx, getAlexaUserQuery, accessToken)
	u.metrics.QueryTime.WithLabelValues("alexa").Observe(time.Since(start).Seconds())

	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("alexa").Inc()
		return nil, fmt.Errorf("querying database: %w", err)
	}

	defer rows.Close() //nolint:errcheck

	user := DefaultUser()

	if !rows.Next() {
		err = rows.Err()
		if err != nil {
			u.metrics.QueryErrors.WithLabelValues("alexa").Inc()
			return nil, fmt.Errorf("iterating database rows: %w", err)
		}

		u.metrics.QueryMissing.WithLabelValues("alexa").Inc()

		return user, ErrNoUser
	}

	devAllowed := "0"

	err = rows.Scan(&user.APIKey, &devAllowed, &user.Environment, &user.Username, &user.UserID)
	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("alexa").Inc()
		return nil, fmt.Errorf("scanning database rows: %w", err)
	}

	err = rows.Err()
	if err != nil {
		u.metrics.QueryErrors.WithLabelValues("alexa").Inc()
		u.Printf("[ERROR] iterating database rows (ignored): %v", err)
	}

	if devAllowed != "1" {
		user.Environment = DefaultEnvironment
	}

	return user, nil
}
