package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds tables.current_hand only after the hands collection exists, to
// avoid a chicken-and-egg dependency at table-creation time.
func init() {
	m.Register(func(app core.App) error {
		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		hands, err := app.FindCollectionByNameOrId("hands")
		if err != nil {
			return err
		}

		tables.Fields.Add(&core.RelationField{
			Name:         "current_hand",
			CollectionId: hands.Id,
			MaxSelect:    1,
		})

		return app.Save(tables)
	}, func(app core.App) error {
		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		tables.Fields.RemoveByName("current_hand")
		return app.Save(tables)
	})
}
