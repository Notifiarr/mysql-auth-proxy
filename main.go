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
		b, err := ioutil.ReadFile(fileName)
		if err != nil {
			log.Printf("ERROR: %v", err)
			return
		}

		pass = string(bytes.TrimSpace(b))
	}

	config := &webserver.Config{
		ListenAddr: listen,
		Config: &userinfo.Config{
			Host: os.Getenv("MYSQL_HOST"),
			User: os.Getenv("MYSQL_USER"),
			Pass: pass,
			Name: os.Getenv("MYSQL_NAME"),
		},
	}

	if err := webserver.Start(config); err != nil {
		log.Printf("ERROR: %v", err)
	}
}
