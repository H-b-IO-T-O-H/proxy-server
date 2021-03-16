package main

import (
	"flag"
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/constants"
	"github.com/H-b-IO-T-O-H/proxy-server/app/server"
	yamlConfig "github.com/rowdyroad/go-yaml-config"
	"log"
	"strings"
)

func main() {
	var config server.Config
	var maxLength uint
	var method string
	var save bool

	flag.StringVar(&method, "method", constants.AllMethods, "Specify http method for save like method='post'")
	flag.UintVar(&maxLength, "length", 100, "Specify max uri length like length='150'")
	flag.BoolVar(&save, "save", true, "Specify save option on database (if false your session won't be saved)")
	flag.Parse()
	config.FindMethod = strings.ToUpper(method)
	config.MaxLength = maxLength
	config.SessionSave = save
	if method != constants.AllMethods && !strings.Contains(constants.AllowedMethods, config.FindMethod) {
		log.Fatal(fmt.Sprintf("Error: Allowed methods: %s", constants.AllowedMethods))
	}
	_ = yamlConfig.LoadConfig(&config, "configs/config.yaml", nil)
	app, err := server.NewServer(config)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error: %s", err.Error()))
	}
	defer app.Close()
	app.Run()
}
