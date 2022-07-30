package userinfo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql" // We use mysql driver, this is how it's loaded.
)

const (
	getUserQuery = "SELECT `developmentEnv`,`environment`,`name`,`id` FROM `users` WHERE `apikey`='%[1]s' " +
		"OR `id`=(SELECT `user_id` FROM `apikeys` WHERE `apikey`='%[1]s' LIMIT 1);"
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

// UI provides an intoerface to query a database for user info.
type UI struct {
	config *Config
	dbase  *sql.DB
}

// UserInfo is the data returned for each user request.
type UserInfo struct {
	Environment string `json:"environment"`
	Username    string `json:"username"`
	UserID      string `json:"userId"`
}

// Errors returns by this package.
var (
	ErrNoConfig = fmt.Errorf("config must contain all fields")
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

// Copy returns a copy of a user's info.
func (u *UserInfo) Copy() *UserInfo {
	if u == nil {
		return DefaultUser()
	}

	return &UserInfo{
		Environment: u.Environment,
		Username:    u.Username,
		UserID:      u.UserID,
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
		return user, nil
	}

	var devAllowed string

	err = rows.Scan(&devAllowed, &user.Environment, &user.Username, &user.UserID)
	if err != nil {
		return nil, fmt.Errorf("scanning database rows: %w", err)
	}

	if devAllowed == "0" {
		user.Environment = DefaultEnvironment
	}

	return user, nil
}
