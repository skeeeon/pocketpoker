package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		hands, err := app.FindCollectionByNameOrId("hands")
		if err != nil {
			return err
		}
		seats, err := app.FindCollectionByNameOrId("seats")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		col := core.NewBaseCollection("hand_players")

		col.Fields.Add(&core.RelationField{
			Name:          "hand",
			Required:      true,
			CollectionId:  hands.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		})
		col.Fields.Add(&core.RelationField{
			Name:         "seat",
			Required:     true,
			CollectionId: seats.Id,
			MaxSelect:    1,
		})
		// user is denormalized so the API rule below can match
		// @request.auth.id without joining through seat.
		col.Fields.Add(&core.RelationField{
			Name:         "user",
			Required:     true,
			CollectionId: users.Id,
			MaxSelect:    1,
		})

		col.Fields.Add(&core.JSONField{Name: "hole_cards", MaxSize: 1024})

		col.Fields.Add(&core.SelectField{
			Name:      "status",
			Required:  true,
			Values:    []string{"active", "folded", "all_in"},
			MaxSelect: 1,
		})

		col.Fields.Add(&core.AutodateField{Name: "created", System: true, OnCreate: true})
		col.Fields.Add(&core.AutodateField{Name: "updated", System: true, OnCreate: true, OnUpdate: true})

		col.AddIndex("idx_hand_players_hand", false, "hand", "")
		col.AddIndex("idx_hand_players_user", false, "user", "")

		// Privacy-critical rule: a row is visible only when either:
		//   (a) the requesting user owns it (their own hole cards), OR
		//   (b) the parent hand has reached showdown (cards revealed).
		//
		// PB re-evaluates list/view rules on every realtime push, so when
		// hand.phase flips to "showdown" subscriptions to opponents'
		// hand_players rows start delivering. This is verified by the
		// Phase 2 privacy matrix (see plan).
		ownerOrShowdown := "@request.auth.id != \"\" && (user = @request.auth.id || hand.phase = \"showdown\")"
		col.ListRule = types.Pointer(ownerOrShowdown)
		col.ViewRule = types.Pointer(ownerOrShowdown)
		// Create/Update/Delete left nil = admin-only.

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("hand_players")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
