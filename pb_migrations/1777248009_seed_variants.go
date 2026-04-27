package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"pocketpoker/engine"
)

// Seeds the variants collection from engine.Variants, the single source
// of truth for variant rules. Idempotent: existing rows (matched by
// `key`) are left untouched. Runs after 1777248008_fix_number_required
// so that zero-valued numeric fields are accepted.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("variants")
		if err != nil {
			return err
		}
		for _, v := range engine.Variants {
			existing, _ := app.FindFirstRecordByData("variants", "key", v.Key)
			if existing != nil {
				continue
			}
			rec := core.NewRecord(col)
			rec.Set("key", v.Key)
			rec.Set("name", v.Name)
			rec.Set("hand_size", v.HandSize)
			rec.Set("min_from_hand", v.MinFromHand)
			rec.Set("max_from_hand", v.MaxFromHand)
			rec.Set("min_from_board", v.MinFromBoard)
			rec.Set("max_from_board", v.MaxFromBoard)
			rec.Set("max_seats", v.MaxSeats)
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		records, err := app.FindRecordsByFilter("variants", "id != \"\"", "", 0, 0)
		if err != nil {
			return err
		}
		for _, r := range records {
			if err := app.Delete(r); err != nil {
				return err
			}
		}
		return nil
	})
}
