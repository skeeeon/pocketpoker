package server

import (
	"io/fs"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterRoutes mounts all custom poker handlers under /api/poker/*.
// If spaFS is non-nil, the embedded Vue SPA is also served at "/" with
// index.html fallback for client-side routes. Pass nil during tests or
// backend-only dev where Vite serves the SPA on its own port.
//
// Accepts any core.App so it works for both the production
// *pocketbase.PocketBase and the test TestApp wrapper.
func RegisterRoutes(app core.App, spaFS fs.FS) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		group := e.Router.Group("/api/poker")
		group.BindFunc(requireAuth)

		group.POST("/tables/{id}/sit", handleSit)
		group.POST("/tables/{id}/leave", handleLeave)
		group.POST("/tables/{id}/delete", handleDeleteTable)
		group.POST("/tables/{id}/add-bot", handleAddBot)
		group.POST("/tables/{id}/remove-bot", handleRemoveBot)
		group.POST("/tables/{id}/ready", handleReady)
		group.POST("/tables/{id}/start-hand", handleStartHand)
		group.POST("/hands/{id}/action", handleAction)
		group.POST("/hands/{id}/fold-player", handleFoldPlayer)
		group.GET("/hands/{id}/replay", handleReplay)

		// Mount the SPA last so /api/* and /_/* (admin) keep priority
		// via more-specific route matching. indexFallback=true serves
		// index.html for unknown paths so vue-router pretty URLs like
		// /tables/abc resolve on direct navigation.
		if spaFS != nil {
			e.Router.GET("/{path...}", apis.Static(spaFS, true))
		}

		return e.Next()
	})
}
