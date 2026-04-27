package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}

		col := core.NewBaseCollection("seats")

		col.Fields.Add(&core.RelationField{
			Name:          "table",
			Required:      true,
			CollectionId:  tables.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		})
		col.Fields.Add(&core.RelationField{
			Name:         "user",
			Required:     true,
			CollectionId: users.Id,
			MaxSelect:    1,
		})
		// seat_number is 0-indexed; PB Required:true would reject 0.
		col.Fields.Add(&core.NumberField{Name: "seat_number", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "stack", OnlyInt: true})
		col.Fields.Add(&core.SelectField{
			Name:      "status",
			Required:  true,
			Values:    []string{"active", "sitting_out", "disconnected"},
			MaxSelect: 1,
		})

		col.Fields.Add(&core.AutodateField{Name: "created", System: true, OnCreate: true})
		col.Fields.Add(&core.AutodateField{Name: "updated", System: true, OnCreate: true, OnUpdate: true})

		col.AddIndex("idx_seats_table_seat_unique", true, "table, seat_number", "")
		col.AddIndex("idx_seats_table_user_unique", true, "table, user", "")

		// Authenticated users may list/view; writes are server-only via custom endpoints.
		authed := "@request.auth.id != \"\""
		col.ListRule = types.Pointer(authed)
		col.ViewRule = types.Pointer(authed)
		// Create/Update/Delete left nil = admin-only (custom endpoints bypass via DAO).

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("seats")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
