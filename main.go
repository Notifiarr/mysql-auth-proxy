package main

import (
	"log"
	"os"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/webserver"
)

const defaultConfigFile = "/config/proxy.conf"

func main() {
	configFile := os.Getenv("AP_CONFIG_FILE")
	if configFile == "" {
		configFile = defaultConfigFile
	}

	if cnfg, err := webserver.LoadConfig(configFile); err != nil {
		log.Fatalf("ERROR: %v", err)
	} else if err := webserver.Start(cnfg); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}
