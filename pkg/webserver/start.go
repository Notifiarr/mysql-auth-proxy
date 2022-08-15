package webserver

import (
	"errors"
	"expvar"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
	apachelog "github.com/lestrrat-go/apache-logformat/v2"
	"golift.io/cache"
	"golift.io/rotatorr"
	"golift.io/rotatorr/timerotator"
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
	LogFile          string `toml:"log_file" xml:"log_file"`
	ErrorFile        string `toml:"error_file" xml:"error_file"`
	*userinfo.Config        // contains mysql host, user, pass, logger.
}

// server holds the running data.
type server struct {
	*Config
	users   *cache.Cache
	servers *cache.Cache
	ui      *userinfo.UI
	exp     *expvar.Map
	httpLog *log.Logger
	server  *http.Server
	errRot  *rotatorr.Logger
	*mux.Router
}

// Start runs the app.
func Start(config *Config) error {
	server := &server{Config: config}

	server.setupLogs()
	server.Println("Auth proxy starting up!")
	server.Printf("DB Host %s, Log: %s, Errors: %s, User: %s, DB Name: %s, Password: %v",
		config.Host, config.LogFile, config.ErrorFile, config.User, config.Name, config.Password != "")

	return server.start()
}

func (s *server) start() error {
	ui, err := userinfo.New(s.Config.Config)
	if err != nil {
		return fmt.Errorf("initializing userinfo: %w", err)
	}

	s.Println("Initialized MySQL successfully")
	s.Printf("HTTP listening at: %s", s.ListenAddr)

	s.users = cache.New(cache.Config{PruneInterval: 3 * time.Minute})
	s.servers = cache.New(cache.Config{})
	s.ui = ui
	s.Router = mux.NewRouter()
	s.exp = exp.GetMap("Incoming HTTP Requests").Init()

	exp.AddVar("Users Cache", expvar.Func(s.users.ExpStats))
	exp.AddVar("Servers Cache", expvar.Func(s.servers.ExpStats))

	return s.startWebServer()
}

func (s *server) startWebServer() error {
	// functions
	s.Use(fixForwardedFor)
	s.Use(s.countRequests)
	// human handlers
	s.HandleFunc("/stats/keys", s.handeUserList).Methods(http.MethodGet)
	s.HandleFunc("/stats/servers", s.handeSrvList).Methods(http.MethodGet)
	s.HandleFunc("/stats/key/{key}", s.handleUserInfo).Methods(http.MethodGet)
	s.HandleFunc("/stats/server/{key}", s.handleSrvInfo).Methods(http.MethodGet)
	s.Handle("/stats", expvar.Handler()).Methods(http.MethodGet)
	// delete handlers
	s.HandleFunc("/auth", s.handleDelKey).Methods(http.MethodDelete).Headers("X-API-Keys", "")
	s.HandleFunc("/auth", s.handleDelSrv).Methods(http.MethodDelete).Headers("X-Server", "")
	// nginx handlers
	s.HandleFunc("/auth", s.handleServer).Methods(http.MethodGet, http.MethodHead).
		Headers("X-Server", "", "X-Password", s.Password)
	s.Handle("/auth", s.parseAPIKey(http.HandlerFunc(s.handleGetKey))).
		Methods(http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut)
	// default: go away
	s.HandleFunc("/", s.noKeyReply)

	// Create pretty Apache-style logs.
	apache, err := apachelog.New(alFmt)
	if err != nil {
		return fmt.Errorf("http log failed: %w", err)
	}

	s.server = &http.Server{
		Addr:              s.ListenAddr,
		Handler:           apache.Wrap(s.Router, s.httpLog.Writer()),
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    9999,
		ErrorLog:          s.Logger,
	}
	if err = s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("cannot start web server: %w", err)
	}

	return nil
}

func (s *server) setupLogs() {
	if s.LogFile != "" {
		s.httpLog = log.New(rotatorr.NewMust(&rotatorr.Config{
			Filepath: s.LogFile,        // log file name.
			FileSize: 10 * 1024 * 1024, // 10 meg
			FileMode: 0o644,            // set file mode.
			Rotatorr: &timerotator.Layout{
				FileCount: 20, // number of files to keep.
			},
		}), "", 0)
	} else {
		s.httpLog = log.New(os.Stdout, "", log.LstdFlags)
	}

	if s.Logger != nil {
		return
	}

	if s.ErrorFile == "" {
		s.Logger = log.New(os.Stderr, "", log.LstdFlags)
		return
	}

	s.errRot = rotatorr.NewMust(&rotatorr.Config{
		Filepath: s.ErrorFile,     // log file name.
		FileSize: 5 * 1024 * 1024, // 5 meg
		FileMode: 0o644,           // set file mode.
		Rotatorr: &timerotator.Layout{
			FileCount:  10, // number of files to keep.
			PostRotate: s.rotateErrLog,
		},
	})
	s.Logger = log.New(s.errRot, "", log.LstdFlags)
	s.rotateErrLog("", "")
}

func (s *server) rotateErrLog(_, _ string) {
	os.Stderr = s.errRot.File
	log.SetOutput(s.errRot)
}
