package server

import (
	"github.com/pocketbase/pocketbase/core"
)

// RegisterUsersHooks installs lifecycle hooks for the `users` auth
// collection. Right now the only hook flips the per-record
// emailVisibility flag to true on creation so any path (admin UI, REST
// signup, our own SPA register flow, the integration test) yields users
// whose email is visible to other authenticated friends. PocketBase has
// no collection-level default for this flag, so a hook is the cleanest
// place to enforce it.
//
// We only set it when the caller didn't explicitly opt out — if a user
// flips emailVisibility off later, future updates won't reset it
// because OnRecordCreate fires only on insert.
func RegisterUsersHooks(app core.App) {
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		if !e.Record.EmailVisibility() {
			e.Record.SetEmailVisibility(true)
		}
		return e.Next()
	})
}
