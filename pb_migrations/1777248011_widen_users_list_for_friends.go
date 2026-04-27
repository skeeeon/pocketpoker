package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Default PB users collection rule restricts list/view to "id = own".
// In a friend-group poker app each player needs to see who their
// opponents are, so widen the rule to any authenticated user. The
// SDK still respects per-user emailVisibility=false for hiding email.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		authed := "@request.auth.id != \"\""
		col.ListRule = types.Pointer(authed)
		col.ViewRule = types.Pointer(authed)
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		owner := "id = @request.auth.id"
		col.ListRule = types.Pointer(owner)
		col.ViewRule = types.Pointer(owner)
		return app.Save(col)
	})
}
