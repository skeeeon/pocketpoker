package main

import (
	"log"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	_ "pocketpoker/pb_migrations"
	"pocketpoker/server"
	"pocketpoker/web"
)

func main() {
	app := pocketbase.New()

	// Register the migration plugin. Automigrate writes .go scaffolds into
	// pb_migrations/ when collections are edited via the admin UI; we
	// only want that during `go run` for local dev, never in a built
	// binary deployed to a VPS.
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: isGoRun,
		Dir:         "pb_migrations",
	})

	server.RegisterRoutes(app, web.Dist())
	server.RegisterUsersHooks(app)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
