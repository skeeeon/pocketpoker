package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"pocketpoker/engine"
)

// Two changes to the variants seed table that must be reflected on
// already-deployed databases (1777248009_seed_variants is idempotent
// and won't re-touch existing rows):
//
//  1. Portland's rule was originally seeded as 1-4 from hand / 1-4
//     from board. The intended rule is 0-4 from hand / 1-5 from board
//     (Miami minus the 5-from-hand option). Update the existing row.
//  2. Insert two new variants — "three_fifths" (5-card hand, 0-3 from
//     hand) and "st_louis" (6-card hand, 0-3 from hand). Their rule
//     fields are read from engine.Variants so this migration stays the
//     single place where keys/values must change in lockstep.
//
// Down: revert Portland to 1/4 and delete the two new rows.
func init() {
	m.Register(func(app core.App) error {
		// 1. Update Portland.
		portland, err := engine.VariantByKey("portland")
		if err != nil {
			return err
		}
		if rec, _ := app.FindFirstRecordByData("variants", "key", "portland"); rec != nil {
			rec.Set("min_from_hand", portland.MinFromHand)
			rec.Set("max_from_hand", portland.MaxFromHand)
			rec.Set("min_from_board", portland.MinFromBoard)
			rec.Set("max_from_board", portland.MaxFromBoard)
			rec.Set("max_seats", portland.MaxSeats)
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		// 2. Insert new variants. Idempotent: skip if already present.
		col, err := app.FindCollectionByNameOrId("variants")
		if err != nil {
			return err
		}
		for _, key := range []string{"three_fifths", "st_louis"} {
			if existing, _ := app.FindFirstRecordByData("variants", "key", key); existing != nil {
				continue
			}
			v, err := engine.VariantByKey(key)
			if err != nil {
				return err
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
		// Down. Revert Portland to its original (incorrect) rule and
		// delete the two new variants.
		if rec, _ := app.FindFirstRecordByData("variants", "key", "portland"); rec != nil {
			rec.Set("min_from_hand", 1)
			rec.Set("max_from_hand", 4)
			rec.Set("min_from_board", 1)
			rec.Set("max_from_board", 4)
			rec.Set("max_seats", 8)
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		for _, key := range []string{"three_fifths", "st_louis"} {
			if rec, _ := app.FindFirstRecordByData("variants", "key", key); rec != nil {
				if err := app.Delete(rec); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
