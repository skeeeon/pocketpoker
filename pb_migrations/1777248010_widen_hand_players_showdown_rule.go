package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The original hand_players list/view rule only revealed opponents'
// hole cards while phase = "showdown". The engine, however, runs the
// showdown logic and transitions to phase = "complete" within a single
// action (no lingering showdown phase). Without widening the rule, the
// frontend cannot render the showdown UI for completed hands. Allow
// reads when the parent hand is at "showdown" OR "complete".
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("hand_players")
		if err != nil {
			return err
		}
		newRule := "@request.auth.id != \"\" && (user = @request.auth.id || hand.phase = \"showdown\" || hand.phase = \"complete\")"
		col.ListRule = types.Pointer(newRule)
		col.ViewRule = types.Pointer(newRule)
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("hand_players")
		if err != nil {
			return err
		}
		oldRule := "@request.auth.id != \"\" && (user = @request.auth.id || hand.phase = \"showdown\")"
		col.ListRule = types.Pointer(oldRule)
		col.ViewRule = types.Pointer(oldRule)
		return app.Save(col)
	})
}
