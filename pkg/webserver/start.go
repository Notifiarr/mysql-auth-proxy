package webserver

import (
	"errors"
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
	alFmt = `%{X-Forwarded-For}i "%{X-Username}o" %{X-UserID}o %t "%r" %>s %b "%{Referer}i" "%{User-agent}i" ` +
		`query:%{X-Request-Time}o req:%{ms}Tms age:%{Age}o env:%{X-Environment}o`
)

// Config is the input data for the server.
type Config struct {
	ListenAddr       string `toml:"listen_addr" xml:"listen_addr"`
	*userinfo.Config        // contains mysql host, user, pass.
}

// server holds the running data.
type server struct {
	cache  *cache.Cache
	config *Config
	*mux.Router
}

// Start runs the app.
func Start(config *Config) error {
	log.Println("Auth proxy starting up!")
	log.Printf("DB Host %s, User: %s, DB Name: %s", config.Host, config.User, config.Name)

	cache, err := cache.New(config.Config)
	if err != nil {
		return fmt.Errorf("cache failure: %w", err)
	}

	defer cache.Stop(false)

	log.Println("Initialized Cache and MySQL successfully")
	log.Printf("HTTP listening at: %s", config.ListenAddr)

	server := &server{
		cache:  cache,
		config: config,
		Router: mux.NewRouter(),
	}

	return server.startWebServer()
}

func (s *server) startWebServer() error {
	// functions
	s.Use(fixForwardedFor)
	s.Use(s.parseAPIKey)
	// handlers
	s.HandleFunc("/auth", s.handleDelKey).Methods(http.MethodDelete)
	s.HandleFunc("/auth", s.handleGetKey)
	s.HandleFunc("/auth/", s.handleGetKey)
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
