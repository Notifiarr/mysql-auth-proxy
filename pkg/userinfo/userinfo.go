package userinfo

import (
	"database/sql"
	"errors"
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
	DefaultUsername    = "-"
	DefaultUserID      = "-1"
)

// Config to get user data from the mysql database.
type Config struct {
	*log.Logger `json:"-"`

	Host string `json:"host" toml:"host" xml:"host"`
	User string `json:"user" toml:"user" xml:"user"`
	Pass string `json:"-"    toml:"pass" xml:"pass"`
	Name string `json:"name" toml:"name" xml:"name"`
}

// UI provides an interface to query a database for user info.
type UI struct {
	*log.Logger

	config  *Config
	dbase   *sql.DB
	exp     *expvar.Map
	metrics *exp.Metrics
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
	ErrNoConfig = errors.New("config must contain all fields")
	ErrNoUser   = errors.New("user not found")
)

// New returns a User Info interface.
func New(config *Config, metrics *exp.Metrics) (*UI, error) {
	if config == nil {
		return nil, ErrNoConfig
	}

	info := &UI{
		metrics: metrics,
		config:  config,
		exp:     exp.GetMap("Outgoing MySQL Requests").Init(),
		Logger:  config.Logger,
	}

	if info.Logger == nil {
		info.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	return info, info.Open()
}

// Open a mysql database connection.
func (u *UI) Open() error {
	if u.dbase != nil {
		_ = u.dbase.Close()
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
	_ = u.dbase.Close()
}

// DefaultUser returns an empty user with default values.
func DefaultUser() *UserInfo {
	return &UserInfo{
		Environment: DefaultEnvironment,
		Username:    DefaultUsername,
		UserID:      DefaultUserID,
	}
}
