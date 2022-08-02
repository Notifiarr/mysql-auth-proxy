package userinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql" // We use mysql driver, this is how it's loaded.
)

const (
	getUserQuery = "SELECT `developmentEnv`,`environment`,`name`,`id` FROM `users` WHERE `apikey`='%[1]s' " +
		"OR `id`=(SELECT `user_id` FROM `apikeys` WHERE `apikey`='%[1]s' LIMIT 1) LIMIT 1;"
	getServerQuery = "SELECT `apikey`,`developmentEnv`,`environment`,`name`,`users`.`id`,CONVERT(FROM_BASE64(`discord`) USING utf8) FROM `users` " +
		"LEFT JOIN `user_settings` ON (`users`.`id` = `user_id`) WHERE `discordServers` LIKE '%%%[1]s%%' AND discord <> '' AND apikey <> '';"
)

// Default user values.
const (
	DefaultEnvironment = "live"
	DefaultUsername    = ""
	DefaultUserID      = "-1"
)

// Config to get user data from the mysql database.
type Config struct {
	Host string `toml:"host" xml:"host"`
	User string `toml:"user" xml:"user"`
	Pass string `toml:"pass" xml:"pass"`
	Name string `toml:"name" xml:"name"`
}

// UI provides an interface to query a database for user info.
type UI struct {
	config *Config
	dbase  *sql.DB
}

// UserInfo is the data returned for each user request.
type UserInfo struct {
	APIKey      string `json:"apiKey,omitempty"`
	Environment string `json:"environment"`
	Username    string `json:"username"`
	UserID      string `json:"userId"`
}

// Errors returned by this package.
var (
	ErrNoConfig = fmt.Errorf("config must contain all fields")
	ErrNoUser   = fmt.Errorf("user not found")
)

// New returns a User Info interface.
func New(config *Config) (*UI, error) {
	if config == nil {
		return nil, ErrNoConfig
	}

	ui := &UI{config: config}

	return ui, ui.Open()
}

// Open a mysql database connection.
func (u *UI) Open() error {
	if u.dbase != nil {
		u.dbase.Close()
	}

	host := "@tcp(" + u.config.Host + ")"
	if strings.HasPrefix(u.config.Host, "@") {
		host = u.config.Host
	}

	dbase, err := sql.Open("mysql", u.config.User+":"+u.config.Pass+host+"/"+u.config.Name)
	if err != nil {
		return fmt.Errorf("mysql server %s: connecting: %w", u.config.Host, err)
	}

	u.dbase = dbase

	return nil
}

// Close the database connection.
func (u *UI) Close() {
	u.dbase.Close()
}

// DefaultUser returns an empty user with default values.
func DefaultUser() *UserInfo {
	return &UserInfo{
		Environment: DefaultEnvironment,
		Username:    DefaultUsername,
		UserID:      DefaultUserID,
	}
}

// GetInfo returns a user's info from a mysql database.
func (u *UI) GetInfo(ctx context.Context, requestKey string) (*UserInfo, error) {
	rows, err := u.dbase.QueryContext(ctx, fmt.Sprintf(getUserQuery, requestKey))
	if err != nil {
		return nil, fmt.Errorf("querying database: %w", err)
	} else if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("getting database rows: %w", err)
	}
	defer rows.Close()

	user := DefaultUser()
	if !rows.Next() {
		return user, ErrNoUser // must return default user on error.
	}

	devAllowed := "0"

	err = rows.Scan(&devAllowed, &user.Environment, &user.Username, &user.UserID)
	if err != nil {
		return nil, fmt.Errorf("scanning database rows: %w", err)
	}

	if devAllowed != "1" {
		user.Environment = DefaultEnvironment
	}

	user.APIKey = requestKey

	return user, nil
}

func (u *UI) GetServer(ctx context.Context, serverID string) (*UserInfo, error) {
	rows, err := u.dbase.QueryContext(ctx, fmt.Sprintf(getServerQuery, serverID))
	if err != nil {
		return nil, fmt.Errorf("querying database: %w", err)
	} else if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("getting database rows: %w", err)
	}
	defer rows.Close()

	var errs error

	for rows.Next() {
		user := DefaultUser()
		devAllowed := "0"
		discord := ""

		err := rows.Scan(&user.APIKey, &devAllowed, &user.Environment, &user.Username, &user.UserID, &discord)
		if err != nil {
			log.Printf("[ERROR] scanning mysql rows: %v", errs)
			continue
		}

		if devAllowed != "1" {
			user.Environment = DefaultEnvironment
		}

		discordVal := struct {
			Server string `json:"discordServer"`
		}{}

		if err = json.Unmarshal([]byte(discord), &discordVal); err != nil {
			log.Printf("[ERROR] mysql json parse: %v", err)
		}

		if discordVal.Server == serverID {
			return user, nil
		}
	}

	return DefaultUser(), nil
}
