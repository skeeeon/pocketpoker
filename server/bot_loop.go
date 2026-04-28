package server

import (
	"errors"
	"math/rand"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"pocketpoker/engine"
)

// thinkMinMs / thinkRangeMs control the random "thinking" delay before
// a bot acts. Just enough variance that bot turns don't feel robotic;
// short enough that play stays brisk.
const (
	thinkMinMs   = 500
	thinkRangeMs = 1500
)

// RegisterBotHooks wires PocketBase record hooks so that whenever a
// hand record is created or updated, we asynchronously check whether
// any bot seat needs to act (or, if the hand just completed, mark its
// bot seats ready and sit out any that busted). Each event spawns its
// own goroutine — fire-and-forget. The hook itself returns immediately
// so the originating transaction commits without waiting on bot logic.
//
// The PB hook fires after the originating transaction commits
// (verified against PB v0.37.3 core/app.go OnRecordAfterUpdateSuccess
// docs), so the goroutine reads a fully-committed state.
//
// Reentrancy: the bot's own SaveHand re-fires this hook, which spawns
// a fresh goroutine. That goroutine reloads the hand, sees that the
// current actor is now a different seat (or that the phase advanced),
// and exits without touching anything. No mutex needed.
func RegisterBotHooks(app core.App) {
	onChange := func(e *core.RecordEvent) error {
		handID := e.Record.Id
		go handleHandChange(app, handID)
		return e.Next()
	}
	app.OnRecordAfterCreateSuccess("hands").BindFunc(onChange)
	app.OnRecordAfterUpdateSuccess("hands").BindFunc(onChange)
}

// handleHandChange is the per-event entry point. Reads the hand record
// and dispatches to the auto-ready or take-bot-action branch.
func handleHandChange(app core.App, handID string) {
	defer func() {
		// A goroutine panic here would crash the whole server. Recover
		// silently — bot logic is best-effort.
		if r := recover(); r != nil {
			app.Logger().Warn("bot loop panic", "err", r, "hand", handID)
		}
	}()

	handRec, err := app.FindRecordById("hands", handID)
	if err != nil {
		return
	}
	tableID := handRec.GetString("table")
	phase := handRec.GetString("phase")

	if phase == "complete" {
		settleBotsAfterHand(app, tableID)
		return
	}
	if phase == "showdown" {
		// Showdown is transient; the engine flips straight to "complete"
		// in the same ApplyAction call. Nothing to do here.
		return
	}

	currentActor := handRec.GetInt("current_actor_seat")
	if currentActor < 0 {
		return
	}

	seatRecs, err := app.FindRecordsByFilter("seats",
		"table = {:t} && seat_number = {:s}",
		"", 1, 0,
		dbx.Params{"t": tableID, "s": currentActor})
	if err != nil || len(seatRecs) == 0 {
		return
	}
	seatRec := seatRecs[0]
	personalityKey := seatRec.GetString("bot_personality")
	if personalityKey == "" {
		return
	}
	personality, ok := engine.Personalities[personalityKey]
	if !ok {
		return
	}

	// Random thinking delay so the bot doesn't slam an action down 1ms
	// after the previous actor's commit.
	delay := time.Duration(thinkMinMs+rand.Intn(thinkRangeMs)) * time.Millisecond
	time.Sleep(delay)

	runBotTurn(app, handID, currentActor, personality)
}

// runBotTurn loads, decides, and saves a single bot action with one
// retry on optimistic-concurrency conflict. If the state has moved on
// (someone else acted, phase advanced) between the hook firing and our
// LoadHand, we silently exit.
func runBotTurn(app core.App, handID string, seat int, p engine.BotPersonality) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for attempt := 0; attempt < 3; attempt++ {
		state, version, err := LoadHand(app, handID)
		if err != nil {
			return
		}
		if state.Phase >= engine.PhaseShowdown {
			return
		}
		if state.CurrentActorSeat != seat {
			return
		}

		action, amount := engine.Decide(state, seat, p, rng)
		newState, err := engine.ApplyAction(state, seat, action, amount)
		if err != nil {
			// Decide returned something the engine refused. This is a
			// bug in Decide, but rather than wedge the table, fold so
			// the hand can advance. Logged so we can investigate.
			app.Logger().Warn("bot decide produced illegal action",
				"hand", handID, "seat", seat, "type", action, "amount", amount, "err", err)
			newState, err = engine.ApplyAction(state, seat, engine.ActionFold, 0)
			if err != nil {
				return
			}
		}

		if err := SaveHand(app, handID, newState, version); err == nil {
			return
		} else if !errors.Is(err, ErrVersionMismatch) {
			app.Logger().Warn("bot SaveHand failed",
				"hand", handID, "seat", seat, "err", err)
			return
		}
		// Version mismatch — someone else wrote first. Loop and retry;
		// the next LoadHand likely sees a different actor and we exit.
	}
}

// settleBotsAfterHand runs once on every transition into PhaseComplete.
// For every bot seat at this table:
//   - if its post-hand stack is 0, flip status to "sitting_out" so
//     start-hand skips it (it can't post blinds);
//   - otherwise mark ready_for_next so the next start-hand isn't
//     gated on bot consent.
//
// Idempotent: repeated invocations no-op when nothing needs changing.
func settleBotsAfterHand(app core.App, tableID string) {
	bots, err := app.FindRecordsByFilter("seats",
		"table = {:t} && bot_personality != ''",
		"", 0, 0,
		dbx.Params{"t": tableID})
	if err != nil {
		return
	}
	for _, bot := range bots {
		stack := bot.GetInt("stack")
		status := bot.GetString("status")

		dirty := false
		if stack <= 0 {
			if status == "active" {
				bot.Set("status", "sitting_out")
				dirty = true
			}
		} else if !bot.GetBool("ready_for_next") {
			bot.Set("ready_for_next", true)
			dirty = true
		}
		if dirty {
			if err := app.Save(bot); err != nil {
				app.Logger().Warn("bot settle save failed",
					"seat", bot.Id, "err", err)
			}
		}
	}
}
