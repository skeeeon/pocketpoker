package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds an optional `name` text field to the users collection so players
// can pick a display name. The avatar field is added via the admin UI
// in this project; this migration is idempotent and only touches `name`.
//
// At display time, code falls back to email (or email prefix) when name
// is empty, so this is safe to apply against accounts created before
// the field existed — they keep working with no backfill required.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if col.Fields.GetByName("name") == nil {
			col.Fields.Add(&core.TextField{
				Name: "name",
				Max:  60,
			})
		}
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("name")
		return app.Save(col)
	})
}
