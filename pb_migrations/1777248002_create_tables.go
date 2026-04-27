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

		col := core.NewBaseCollection("tables")

		col.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 100})
		col.Fields.Add(&core.RelationField{
			Name:         "created_by",
			Required:     true,
			CollectionId: users.Id,
			MaxSelect:    1,
		})
		// Required:true on NumberField means "non-zero"; we do server-side
		// validation in the create-table endpoint instead.
		col.Fields.Add(&core.NumberField{Name: "buy_in", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "small_blind", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "big_blind", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "max_seats", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "current_dealer_seat", OnlyInt: true})
		col.Fields.Add(&core.SelectField{
			Name:      "status",
			Required:  true,
			Values:    []string{"waiting", "active"},
			MaxSelect: 1,
		})

		col.Fields.Add(&core.AutodateField{Name: "created", System: true, OnCreate: true})
		col.Fields.Add(&core.AutodateField{Name: "updated", System: true, OnCreate: true, OnUpdate: true})

		// Auth users may list/view/create. Only the creator can update or delete.
		authed := "@request.auth.id != \"\""
		creator := "@request.auth.id != \"\" && @request.auth.id = created_by"
		col.ListRule = types.Pointer(authed)
		col.ViewRule = types.Pointer(authed)
		col.CreateRule = types.Pointer(authed)
		col.UpdateRule = types.Pointer(creator)
		col.DeleteRule = types.Pointer(creator)

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
