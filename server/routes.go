package server

import (
	"github.com/pocketbase/pocketbase/core"
)

// RegisterRoutes mounts all custom poker handlers under /api/poker/*.
// Accepts any core.App so it works for both the production
// *pocketbase.PocketBase and the test TestApp wrapper.
func RegisterRoutes(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		group := e.Router.Group("/api/poker")
		group.BindFunc(requireAuth)

		group.POST("/tables/{id}/sit", handleSit)
		group.POST("/tables/{id}/leave", handleLeave)
		group.POST("/tables/{id}/ready", handleReady)
		group.POST("/tables/{id}/start-hand", handleStartHand)
		group.POST("/hands/{id}/action", handleAction)
		group.POST("/hands/{id}/fold-player", handleFoldPlayer)
		group.GET("/hands/{id}/replay", handleReplay)

		return e.Next()
	})
}
