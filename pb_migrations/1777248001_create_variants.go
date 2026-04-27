package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		col := core.NewBaseCollection("variants")

		// PB treats Required:true on NumberField as "non-zero", which would
		// reject legitimate zeros (e.g. min_from_hand=0 for Hold'em). The
		// fields are validated server-side by the engine instead.
		col.Fields.Add(&core.TextField{Name: "key", Required: true})
		col.Fields.Add(&core.TextField{Name: "name", Required: true})
		col.Fields.Add(&core.NumberField{Name: "hand_size", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "min_from_hand", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "max_from_hand", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "min_from_board", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "max_from_board", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "max_seats", OnlyInt: true})

		col.Fields.Add(&core.AutodateField{Name: "created", System: true, OnCreate: true})
		col.Fields.Add(&core.AutodateField{Name: "updated", System: true, OnCreate: true, OnUpdate: true})

		col.AddIndex("idx_variants_key_unique", true, "key", "")

		// Authenticated read; writes only via custom endpoints (or admin).
		authed := "@request.auth.id != \"\""
		col.ListRule = types.Pointer(authed)
		col.ViewRule = types.Pointer(authed)
		// Create/Update/Delete left nil = admin-only.

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("variants")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
