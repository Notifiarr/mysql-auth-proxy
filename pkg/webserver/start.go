package webserver

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/docs"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golift.io/cache"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
	"golift.io/rotatorr"
	"golift.io/rotatorr/timerotator"
)

const (
	pruneInterval = 3 * time.Minute
	timeout       = 15 * time.Second
)

// Canonical HTTP Headers.
const (
	HeaderXAPIKey       = "X-Api-Key"  //nolint:gosec // not a cred.
	HeaderXAPIKeys      = "X-Api-Keys" //nolint:gosec // not a cred.
	HeaderXOriginalURI  = "X-Original-Uri"
	HeaderXServer       = "X-Server"
	HeaderXUsername     = "X-Username"
	HeaderXUserid       = "X-Userid"
	HeaderXForwardedFor = "X-Forwarded-For"
	HeaderEnvironment   = "X-Environment"
	HeaderContentType   = "Content-Type"
	HeaderAge           = "Age"
)

// Config is the input data for the server.
type Config struct {
	*userinfo.Config // contains mysql host, user, pass, logger.

	ListenAddr  string   `json:"listenAddr"  toml:"listen_addr"   xml:"listen_addr"`
	Password    string   `json:"-"           toml:"password"      xml:"password"`
	LogFile     string   `json:"logFile"     toml:"log_file"      xml:"log_file"`
	ErrorFile   string   `json:"errorFile"   toml:"error_file"    xml:"error_file"`
	NoAuthPaths []string `json:"noAuthPaths" toml:"no_auth_paths" xml:"no_auth_path"`
	// CacheShards is golift.io/cache partition count for users and servers; 0 means library default (single shard).
	CacheShards int    `json:"cacheShards,omitempty" toml:"cache_shards" xml:"cache_shards"`
	filePath    string // path to loaded config file.
}

// server holds the running data.
type server struct {
	*Config

	users   *cache.Cache
	servers *cache.Cache
	ui      *userinfo.UI
	httpLog *log.Logger
	server  *http.Server
	errRot  *rotatorr.Logger
	// noAuthMu protects NoAuthPaths on the embedded Config (RequiresAPIKey, reload, showConfig).
	noAuthMu sync.RWMutex
	metrics  *exp.Metrics
}

// ErrNoSQLConfig is returned if no mysql config is present.
var ErrNoSQLConfig = errors.New("no mysql config present")

// LoadConfig reads in a config file and/or env variables to configure the app.
func LoadConfig(filename string) (*Config, error) { //nolint:cyclop
	config := Config{filePath: filename}

	if filename != "" {
		err := cnfgfile.Unmarshal(&config, filename)
		if err != nil {
			return nil, fmt.Errorf("config file: %w", err)
		}
	}

	_, err := cnfg.UnmarshalENV(&config, "AP")
	if err != nil {
		return nil, fmt.Errorf("environment variables: %w", err)
	}

	if config.Config == nil {
		return nil, ErrNoSQLConfig
	}

	if config.ListenAddr == "" {
		config.ListenAddr = "0.0.0.0:8080"
	}

	if fileName := os.Getenv("AP_MYSQL_PASS_FILE"); config.Pass == "" && fileName != "" {
		fileData, err := os.ReadFile(fileName)
		if err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		config.Pass = string(bytes.TrimSpace(fileData))
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
	server.Printf("Cache shards: %d", config.CacheShards)

	return server.start()
}

func (s *server) start() error {
	s.users = cache.New(cache.Config{
		PruneInterval:   pruneInterval,
		RequestAccuracy: time.Second,
		Shards:          s.CacheShards,
	})
	defer s.users.Stop(false)

	s.servers = cache.New(cache.Config{
		RequestAccuracy: time.Second,
		Shards:          s.CacheShards,
	})
	defer s.servers.Stop(false)

	s.metrics = exp.GetMetrics(&exp.CacheCollector{Stats: exp.CacheList{
		"servers": s.servers.Stats,
		"users":   s.users.Stats,
	}})

	info, err := userinfo.New(s.Config.Config, s.metrics)
	if err != nil {
		return fmt.Errorf("initializing userinfo: %w", err)
	}

	s.Println("Initialized MySQL successfully")
	s.Printf("HTTP listening at: %s", s.ListenAddr)

	s.ui = info

	return s.startWebServer()
}

func (s *server) startWebServer() error {
	mux := http.NewServeMux()
	docsHandler := http.StripPrefix("/docs/", http.FileServer(docs.AssetFS()))
	mux.Handle("GET /docs/", docsHandler)
	mux.Handle("HEAD /docs/", docsHandler)
	mux.HandleFunc("GET /swagger.json", s.handlerSwaggerDoc)
	mux.HandleFunc("GET /reload", s.reloadConfig)
	mux.HandleFunc("GET /stats/config", s.showConfig)
	mux.HandleFunc("GET /stats/keys", s.handeUserList)
	mux.HandleFunc("GET /stats/servers", s.handeSrvList)
	mux.HandleFunc("GET /stats/key/{key}", s.handleUserInfo)
	mux.HandleFunc("GET /stats/server/{key}", s.handleSrvInfo)
	mux.HandleFunc("/auth", s.handleAuth)
	mux.Handle("GET /metrics", promhttp.Handler())

	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
		http.MethodConnect, http.MethodTrace,
	} {
		mux.HandleFunc(method+" /{$}", s.noKeyReply)
	}

	s.server = &http.Server{
		Addr:              s.ListenAddr,
		Handler:           s.accessLogWrap(mux, s.httpLog.Writer()),
		ReadTimeout:       timeout,
		ReadHeaderTimeout: timeout,
		WriteTimeout:      timeout,
		IdleTimeout:       timeout,
		ErrorLog:          s.Logger,
	}

	err := s.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("cannot start web server: %w", err)
	}

	return nil
}

func (s *server) setupLogs() {
	const (
		logFileSize = 20 * 1024 * 1024 // 20 meg
		keepLogs    = 50
		divisor     = 2 // error log gets the above two values cut in half.
		fileMode    = 0o644
	)

	if s.LogFile != "" {
		s.httpLog = log.New(rotatorr.NewMust(&rotatorr.Config{
			Filepath: s.LogFile, // log file name.
			FileSize: logFileSize,
			FileMode: fileMode, // set file mode.
			Rotatorr: &timerotator.Layout{
				FileCount: keepLogs, // number of files to keep.
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
		Filepath: s.ErrorFile, // log file name.
		FileSize: logFileSize / divisor,
		FileMode: fileMode, // set file mode.
		Rotatorr: &timerotator.Layout{
			FileCount:  keepLogs / divisor, // number of files to keep.
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

// RequiresAPIKey returns true if the requested path requires an api key.
func (s *server) RequiresAPIKey(uriPath string) bool {
	s.noAuthMu.RLock()
	defer s.noAuthMu.RUnlock()

	for _, prefix := range s.NoAuthPaths {
		if strings.HasPrefix(uriPath, prefix) {
			return false
		}
	}

	return true
}
