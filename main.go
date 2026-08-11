package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"wme/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "30120"
	}

	app := api.NewAPI()
	app.SetupRoutes()

	go func() {
		time.Sleep(2 * time.Second)
		app.StartupScan()
	}()

	fs := http.FileServer(http.Dir(filepath.Join(".", "static")))
	http.Handle("/", fs)

	log.Printf("WME Virtualization Manager listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
