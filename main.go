// Package main is the main package for the mysql-auth-proxy application.
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

	cnfg, err := webserver.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("ERROR: %v", err)
	}

	err = webserver.Start(cnfg)
	if err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}
