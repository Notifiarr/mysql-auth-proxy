package userinfo

import (
	"database/sql"
	"expvar"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	_ "github.com/go-sql-driver/mysql" // We use mysql driver, this is how it's loaded.
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
	*log.Logger
}

// UI provides an interface to query a database for user info.
type UI struct {
	config *Config
	dbase  *sql.DB
	exp    *expvar.Map
	*log.Logger
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

	ui := &UI{
		config: config,
		exp:    exp.GetMap("Outgoing MySQL Requests").Init(),
		Logger: config.Logger,
	}

	if ui.Logger == nil {
		ui.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

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
