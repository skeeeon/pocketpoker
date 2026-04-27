package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		tables, err := app.FindCollectionByNameOrId("tables")
		if err != nil {
			return err
		}
		variants, err := app.FindCollectionByNameOrId("variants")
		if err != nil {
			return err
		}

		col := core.NewBaseCollection("hands")

		col.Fields.Add(&core.RelationField{
			Name:          "table",
			Required:      true,
			CollectionId:  tables.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		})
		col.Fields.Add(&core.RelationField{
			Name:         "variant",
			Required:     true,
			CollectionId: variants.Id,
			MaxSelect:    1,
		})

		// 0-indexed seat numbers; PB Required:true rejects 0.
		col.Fields.Add(&core.NumberField{Name: "dealer_seat", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "small_blind_seat", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "big_blind_seat", OnlyInt: true})

		col.Fields.Add(&core.JSONField{Name: "community_cards", MaxSize: 4096})
		col.Fields.Add(&core.NumberField{Name: "pot", OnlyInt: true})

		col.Fields.Add(&core.SelectField{
			Name:      "phase",
			Required:  true,
			Values:    []string{"preflop", "flop", "turn", "river", "showdown", "complete"},
			MaxSelect: 1,
		})

		col.Fields.Add(&core.NumberField{Name: "current_actor_seat", OnlyInt: true})
		col.Fields.Add(&core.NumberField{Name: "current_bet", OnlyInt: true})

		col.Fields.Add(&core.JSONField{Name: "actions", MaxSize: 1 << 20})
		// version is the optimistic-concurrency token; bumped by every server write.
		// Starts at 0 so PB Required:true would block initial inserts.
		col.Fields.Add(&core.NumberField{Name: "version", OnlyInt: true})

		// deck_state is hidden from API responses; only internal/admin reads it.
		col.Fields.Add(&core.JSONField{
			Name:    "deck_state",
			Hidden:  true,
			MaxSize: 4096,
		})

		col.Fields.Add(&core.JSONField{Name: "winner_seats", MaxSize: 8192})

		col.Fields.Add(&core.AutodateField{Name: "created", System: true, OnCreate: true})
		col.Fields.Add(&core.AutodateField{Name: "updated", System: true, OnCreate: true, OnUpdate: true})

		col.AddIndex("idx_hands_table", false, "table", "")

		// Authenticated reads; writes server-only via custom endpoints.
		authed := "@request.auth.id != \"\""
		col.ListRule = types.Pointer(authed)
		col.ViewRule = types.Pointer(authed)
		// Create/Update/Delete left nil.

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("hands")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
