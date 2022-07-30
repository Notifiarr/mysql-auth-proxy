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

// Config is the input data for the server.
type Config struct {
	ListenAddr       string `toml:"listen_addr" xml:"listen_addr"`
	*userinfo.Config        // contains mysql host, user, pass.
}

// server holds the running data.
type server struct {
	cache  *cache.Cache
	config *Config
	router *mux.Router
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

	log.Println("Initialized Cached and MySQL Successfully")
	log.Printf("HTTP Listening At: %s", config.ListenAddr)

	server := &server{
		cache:  cache,
		config: config,
		router: mux.NewRouter(),
	}

	return server.startWebServer()
}

func (s *server) startWebServer() error {
	s.router.Use(fixForwardedFor)
	s.router.Use(parseAPIKey)
	s.router.HandleFunc("/}", noKeyReply)
	s.router.HandleFunc("/auth/", s.handleKey)
	s.router.HandleFunc("/auth", s.handleKey)
	s.router.HandleFunc("/auth/", s.handleDelKey).Methods(http.MethodDelete)
	s.router.HandleFunc("/auth", s.handleDelKey).Methods(http.MethodDelete)
	s.router.HandleFunc("/del/{key}", s.handleDelKey) // deprecate this.

	//nolint:lll // Create pretty Apache-style logs.
	apache, err := apachelog.New(`%{X-Forwarded-For}i ${X-Username}o %{X-UserID}o %t "%r" %>s %b "%{Referer}i" "%{User-agent}i" %{X-Request-Time}o %{ms}Tms`)
	if err != nil {
		return fmt.Errorf("apache log problem: %w", err)
	}

	smx := http.NewServeMux()                         // router magic.
	smx.Handle("/", apache.Wrap(s.router, os.Stderr)) // dump logs into docker container, or whatever.

	err = http.ListenAndServe(s.config.ListenAddr, smx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("cannot start web server: %w", err)
	}

	return nil
}
