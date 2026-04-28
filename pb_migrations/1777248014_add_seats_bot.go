package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds bot-player support to the seats collection. Three changes,
// applied as a single migration so a deploy that's mid-rollback never
// leaves the schema in a half-bot state:
//
//  1. New select field seats.bot_personality. Empty string ⇒ human
//     seat (the default), non-empty ⇒ AI seat with that archetype.
//  2. seats.user becomes optional. Bot seats have no associated user.
//     The unique index on (table, user) is also re-created as a partial
//     index so multiple bots (each with user="") can sit at one table.
//     Without the partial WHERE, SQLite would treat the empty-string
//     user as a real value and reject the second bot insert.
//  3. hand_players.user becomes optional. handleStartHand copies
//     seats.user into hand_players.user; for bot seats that's "", and
//     a Required field would reject the insert.
//
// The hand_players list/view rule (set by 1777248010) compares
// `user = @request.auth.id`. For bot rows where user="", no caller's
// id matches, so opponents' bot hole cards stay hidden until the rule
// also unlocks via `hand.phase = "showdown" || hand.phase = "complete"`.
// No rule change is needed.
func init() {
	m.Register(func(app core.App) error {
		seatsCol, err := app.FindCollectionByNameOrId("seats")
		if err != nil {
			return err
		}

		// 1. Add bot_personality. The empty-string value is the human
		// default and must be in the allowed set so existing rows
		// validate without a backfill.
		seatsCol.Fields.Add(&core.SelectField{
			Name:      "bot_personality",
			Required:  false,
			Values:    []string{"", "tight", "loose", "maniac", "station"},
			MaxSelect: 1,
		})

		// 2a. Flip seats.user to optional.
		if userField, ok := seatsCol.Fields.GetByName("user").(*core.RelationField); ok {
			userField.Required = false
		}

		// 2b. Replace the (table, user) unique index with a partial
		// version that excludes bot rows (user = '').
		seatsCol.AddIndex(
			"idx_seats_table_user_unique",
			true,
			"`table`, `user`",
			"`user` != ''",
		)

		if err := app.Save(seatsCol); err != nil {
			return err
		}

		// 3. Flip hand_players.user to optional.
		hpCol, err := app.FindCollectionByNameOrId("hand_players")
		if err != nil {
			return err
		}
		if userField, ok := hpCol.Fields.GetByName("user").(*core.RelationField); ok {
			userField.Required = false
		}
		return app.Save(hpCol)
	}, func(app core.App) error {
		// Down: reverse all three edits.
		seatsCol, err := app.FindCollectionByNameOrId("seats")
		if err != nil {
			return err
		}
		seatsCol.Fields.RemoveByName("bot_personality")
		if userField, ok := seatsCol.Fields.GetByName("user").(*core.RelationField); ok {
			userField.Required = true
		}
		seatsCol.AddIndex(
			"idx_seats_table_user_unique",
			true,
			"`table`, `user`",
			"",
		)
		if err := app.Save(seatsCol); err != nil {
			return err
		}

		hpCol, err := app.FindCollectionByNameOrId("hand_players")
		if err != nil {
			return err
		}
		if userField, ok := hpCol.Fields.GetByName("user").(*core.RelationField); ok {
			userField.Required = true
		}
		return app.Save(hpCol)
	})
}
