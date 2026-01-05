package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Zeglius/yafti-go/internal/consts"
	srv "github.com/Zeglius/yafti-go/server"
	"golang.org/x/sync/errgroup"
)

//go:generate go tool templ generate

//go:embed static/**
var static embed.FS

func main() {
	// Get the wrapper command from environment variables
	// If YAFTI_EXEC_WRAPPER is set, the server will be started and the wrapper command will be executed
	cmd := os.Getenv("YAFTI_EXEC_WRAPPER")

	// Instantiate server
	server := srv.New()

	// Load static assets
	server.StaticAssets = &static

	// If no wrapper command is provided, just run the server directly...
	if cmd == "" {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Panic(err)
		}
		return
	}

	// ... else, we start the server and execute the wrapper command.
	cmd = strings.ReplaceAll(cmd, "%u", "http://localhost:"+consts.PORT)

	// Start the server and start the server wrapper command, alongside
	// executing the wrapper command.
	var errg errgroup.Group
	errg.Go(func() error {
		server.Start()
		return nil
	})
	errg.Go(exec.Command("sh", "-c", cmd).Start)

	if err := errg.Wait(); err != nil {
		log.Panic(err)
	}
}
