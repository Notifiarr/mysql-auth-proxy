package webserver

import (
	"bytes"
	"errors"
	"expvar"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/docs"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/gorilla/mux"
	apachelog "github.com/lestrrat-go/apache-logformat/v2"
	"golift.io/cache"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
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
	ListenAddr       string   `toml:"listen_addr" xml:"listen_addr" json:"listenAddr"`
	Password         string   `toml:"password" xml:"password" json:"-"`
	LogFile          string   `toml:"log_file" xml:"log_file" json:"logFile"`
	ErrorFile        string   `toml:"error_file" xml:"error_file" json:"errorFile"`
	NoAuthPaths      []string `toml:"no_auth_paths" xml:"no_auth_path" json:"noAuthPaths"`
	filePath         string   // path to loaded config file.
	*userinfo.Config          // contains mysql host, user, pass, logger.
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
	pathCh  chan string
	addCh   chan []string
	answer  chan bool
	*mux.Router
}

// ErrNoSQLConfig is returned if no mysql config is present.
var ErrNoSQLConfig = fmt.Errorf("no mysql config present")

// LoadConfig reads in a config file and/or env variables to configure the app.
func LoadConfig(filename string) (*Config, error) {
	config := Config{filePath: filename}

	if filename != "" {
		if err := cnfgfile.Unmarshal(&config, filename); err != nil {
			return nil, fmt.Errorf("config file: %w", err)
		}
	}

	if _, err := cnfg.UnmarshalENV(&config, "AP"); err != nil {
		return nil, fmt.Errorf("environment variables: %w", err)
	}

	if config.Config == nil {
		return nil, ErrNoSQLConfig
	}

	if config.ListenAddr == "" {
		config.ListenAddr = "0.0.0.0:8080"
	}

	if fileName := os.Getenv("AP_MYSQL_PASS_FILE"); config.Config.Pass == "" && fileName != "" {
		fileData, err := os.ReadFile(fileName)
		if err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		config.Config.Pass = string(bytes.TrimSpace(fileData))
	}

	if fileName := os.Getenv("AP_SECRET_FILE"); config.Password == "" && fileName != "" {
		fileData, err := os.ReadFile(fileName)
		if err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		config.Password = string(bytes.TrimSpace(fileData))
	}

	return &config, nil
}

// Start runs the app.
func Start(config *Config) error {
	server := &server{Config: config}
	server.setupLogs()
	server.Println("Auth proxy starting up!")
	server.Printf("DB Host %s, Log: %s, Errors: %s, User: %s, DB Name: %s, Password: %v",
		config.Host, config.LogFile, config.ErrorFile, config.User, config.Name, config.Password != "")
	server.Printf("No-Key-Required Paths (%d): %s",
		len(config.NoAuthPaths), strings.Join(config.NoAuthPaths, ", "))

	go server.pathCheck()

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
	s.Use(s.fixRequestURI)
	s.Use(s.countRequests)
	// api docs
	s.PathPrefix("/docs/").Handler(http.StripPrefix("/docs/", http.FileServer(docs.AssetFS())))
	s.HandleFunc("/swagger.json", s.handlerSwaggerDoc).Methods(http.MethodGet)
	// human handlers
	s.HandleFunc("/reload", s.reloadConfig).Methods(http.MethodGet)
	s.HandleFunc("/stats/config", s.showConfig).Methods(http.MethodGet)
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

// pathCheck runs in a go routine and handles path checks and adding new paths.
// When the reload handler is hit it throws new (or existing) no-auth paths into this loop.
func (s *server) pathCheck() {
	s.pathCh = make(chan string)
	s.addCh = make(chan []string)
	s.answer = make(chan bool)
	checkPath := func(requestURI string) bool {
		for _, prefix := range s.NoAuthPaths {
			if strings.HasPrefix(requestURI, prefix) {
				return true
			}
		}

		return false
	}

	for {
		select {
		case checkpath := <-s.pathCh:
			s.answer <- checkPath(checkpath)
		case setPaths := <-s.addCh:
			s.NoAuthPaths = setPaths
			s.answer <- true
		}
	}
}

// RequiresAPIKey returns true if the requested path requires an api key.
func (s *server) RequiresAPIKey(uriPath string) bool {
	s.pathCh <- uriPath
	return !<-s.answer
}
