package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// PocketBase's NumberField.Required maps to "value must be non-zero",
// which incorrectly rejects legitimate zeros (seat 0, min_from_hand=0,
// pot=0, version=0, etc). The earlier collection-create migrations
// were authored with Required:true on these fields, so update existing
// schemas to remove that constraint. Validation is enforced by the
// engine and custom HTTP handlers instead.
//
// Newly-deployed environments running the fixed migration files won't
// hit this — those create collections without Required:true. The fix
// is therefore idempotent: it only flips Required to false where it
// was set, and leaves correct schemas untouched.
func init() {
	m.Register(func(app core.App) error {
		fixes := map[string][]string{
			"variants": {
				"hand_size", "min_from_hand", "max_from_hand",
				"min_from_board", "max_from_board", "max_seats",
			},
			"tables": {
				"buy_in", "small_blind", "big_blind", "max_seats",
				"current_dealer_seat",
			},
			"seats": {
				"seat_number", "stack",
			},
			"hands": {
				"dealer_seat", "small_blind_seat", "big_blind_seat",
				"pot", "current_actor_seat", "current_bet", "version",
			},
		}
		for colName, fieldNames := range fixes {
			col, err := app.FindCollectionByNameOrId(colName)
			if err != nil {
				return err
			}
			changed := false
			for _, fn := range fieldNames {
				f := col.Fields.GetByName(fn)
				if f == nil {
					continue
				}
				if nf, ok := f.(*core.NumberField); ok && nf.Required {
					nf.Required = false
					changed = true
				}
			}
			if changed {
				if err := app.Save(col); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(app core.App) error {
		// No-op down: re-imposing Required:true would break valid zeros
		// already in the data.
		return nil
	})
}
