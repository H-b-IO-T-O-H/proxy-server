package main

import (
	"fmt"
	api "github.com/H-b-IO-T-O-H/proxy-server/app"
	yamlConfig "github.com/rowdyroad/go-yaml-config"
	"log"
)

func main() {
	var config api.Config

	_ = yamlConfig.LoadConfig(&config, "configs/config.yaml", nil)
	app, err := api.NewServer(config)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error: %s", err.Error()))
	}
	defer app.Close()
	app.Run()
}
