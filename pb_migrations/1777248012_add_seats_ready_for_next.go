package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds a per-seat "ready for next hand" flag so the table can pause at
// the end of a hand for players to review the result before the dealer
// deals again. Reset to false whenever a new hand is dealt.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("seats")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.BoolField{Name: "ready_for_next"})
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("seats")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("ready_for_next")
		return app.Save(col)
	})
}
