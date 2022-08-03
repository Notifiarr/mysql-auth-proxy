package webserver

import (
	"errors"
	"expvar"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/cache"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
	apachelog "github.com/lestrrat-go/apache-logformat/v2"
)

const (
	// Apache Log format.
	alFmt = `%V %{X-Forwarded-For}i "%{X-Username}o" %{X-UserID}o %t "%r" %>s %b "%{Referer}i" "%{User-agent}i" ` +
		`query:%{X-Request-Time}o req:%{ms}Tms age:%{Age}o env:%{X-Environment}o key:%{X-Key}o(%{X-Length}o) "srv:%{X-Server}i"`
)

// Config is the input data for the server.
type Config struct {
	ListenAddr       string `toml:"listen_addr" xml:"listen_addr"`
	Password         string `toml:"password" xml:"password"`
	*userinfo.Config        // contains mysql host, user, pass.
}

// server holds the running data.
type server struct {
	users   *cache.Cache
	servers *cache.Cache
	config  *Config
	ui      *userinfo.UI
	*mux.Router
}

// Start runs the app.
func Start(config *Config) error {
	log.Println("Auth proxy starting up!")
	log.Printf("DB Host %s, User: %s, DB Name: %s, Password: %v", config.Host, config.User, config.Name, config.Password != "")

	ui, err := userinfo.New(config.Config)
	if err != nil {
		return fmt.Errorf("initializing userinfo: %w", err)
	}

	log.Println("Initialized MySQL successfully")
	log.Printf("HTTP listening at: %s", config.ListenAddr)

	server := &server{
		users:   cache.New("Users", true),
		servers: cache.New("Servers", false),
		config:  config,
		ui:      ui,
		Router:  mux.NewRouter(),
	}

	return server.startWebServer()
}

func (s *server) startWebServer() error {
	// functions
	s.Use(fixForwardedFor)
	s.Use(countRequests)
	// handlers
	s.HandleFunc("/stats", s.handleUserStats).Methods(http.MethodGet).Headers("X-API-Key", "")
	s.HandleFunc("/stats", s.handleSrvStats).Methods(http.MethodGet).Headers("X-Server", "")
	s.Handle("/stats", expvar.Handler()).Methods(http.MethodGet)
	s.HandleFunc("/auth", s.handleDelKey).Methods(http.MethodDelete).Headers("X-API-Keys", "")
	s.HandleFunc("/auth", s.handleDelSrv).Methods(http.MethodDelete).Headers("X-Server", "")
	s.HandleFunc("/auth", s.handleServer).Methods(http.MethodGet, http.MethodHead).
		Headers("X-Server", "", "X-Password", s.config.Password)
	s.Handle("/auth", s.parseAPIKey(http.HandlerFunc(s.handleGetKey))).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut)
	s.HandleFunc("/", s.noKeyReply)

	// Create pretty Apache-style logs.
	apache, err := apachelog.New(alFmt)
	if err != nil {
		return fmt.Errorf("http log failed: %w", err)
	}

	smx := http.NewServeMux()                         // router magic.
	smx.Handle("/", apache.Wrap(s.Router, os.Stderr)) // dump logs into docker container, or whatever.

	err = http.ListenAndServe(s.config.ListenAddr, smx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("cannot start web server: %w", err)
	}

	return nil
}
