package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
	"github.com/Notifiarr/mysql-auth-proxy/pkg/webserver"
)

func main() {
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = "0.0.0.0:8080"
	}

	pass := os.Getenv("MYSQL_PASS")
	if fileName := os.Getenv("MYSQL_PASS_FILE"); pass == "" && fileName != "" {
		fileData, err := ioutil.ReadFile(fileName)
		if err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		pass = string(bytes.TrimSpace(fileData))
	}

	password := os.Getenv("SECRET")
	if fileName := os.Getenv("SECRET_FILE"); password == "" && fileName != "" {
		fileData, err := ioutil.ReadFile(fileName)
		if err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		password = string(bytes.TrimSpace(fileData))
	}

	config := &webserver.Config{
		ListenAddr: listen,
		Password:   password,
		LogFile:    os.Getenv("LOG_FILE"),
		Config: &userinfo.Config{
			Host: os.Getenv("MYSQL_HOST"),
			User: os.Getenv("MYSQL_USER"),
			Pass: pass,
			Name: os.Getenv("MYSQL_NAME"),
		},
	}

	if err := webserver.Start(config); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}
