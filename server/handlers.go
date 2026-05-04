package server

import (
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"pocketpoker/engine"
)

// requireAuth is the middleware applied to every /api/poker/* route.
func requireAuth(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	return e.Next()
}

// ----- Sit -----

type sitRequest struct {
	SeatNumber    int `json:"seat_number"`
	BuyInAmount   int `json:"buy_in_amount"`
}

func handleSit(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")
	var req sitRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if req.SeatNumber < 0 {
		return e.BadRequestError("seat_number must be non-negative", nil)
	}

	tableRec, err := e.App.FindRecordById("tables", tableID)
	if err != nil {
		return e.NotFoundError("table not found", nil)
	}
	maxSeats := tableRec.GetInt("max_seats")
	if req.SeatNumber >= maxSeats {
		return e.BadRequestError(
			fmt.Sprintf("seat_number %d exceeds max_seats %d", req.SeatNumber, maxSeats), nil)
	}
	if req.BuyInAmount < tableRec.GetInt("buy_in") {
		return e.BadRequestError("buy_in_amount below table buy_in", nil)
	}

	// Reject if seat already occupied or user already seated.
	existingBySeat, _ := e.App.FindRecordsByFilter("seats",
		"table = {:t} && seat_number = {:s}", "", 1, 0,
		dbx.Params{"t": tableID, "s": req.SeatNumber})
	if len(existingBySeat) > 0 {
		return e.BadRequestError("seat already taken", nil)
	}
	existingByUser, _ := e.App.FindRecordsByFilter("seats",
		"table = {:t} && user = {:u}", "", 1, 0,
		dbx.Params{"t": tableID, "u": e.Auth.Id})
	if len(existingByUser) > 0 {
		return e.BadRequestError("you are already seated at this table", nil)
	}

	col, err := e.App.FindCollectionByNameOrId("seats")
	if err != nil {
		return e.InternalServerError("seats collection missing", err)
	}
	rec := core.NewRecord(col)
	rec.Set("table", tableID)
	rec.Set("user", e.Auth.Id)
	rec.Set("seat_number", req.SeatNumber)
	rec.Set("stack", req.BuyInAmount)
	rec.Set("status", "active")
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("save seat", err)
	}
	return e.JSON(200, map[string]any{"seat_id": rec.Id})
}

// ----- Leave -----

func handleLeave(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")

	seats, err := e.App.FindRecordsByFilter("seats",
		"table = {:t} && user = {:u}", "", 1, 0,
		dbx.Params{"t": tableID, "u": e.Auth.Id})
	if err != nil {
		return e.InternalServerError("query seats", err)
	}
	if len(seats) == 0 {
		return e.NotFoundError("you are not seated at this table", nil)
	}
	seat := seats[0]
	stack := seat.GetInt("stack")

	// Refuse to leave during an active hand the caller is in. The dealer
	// can force-fold them via /fold-player; once the hand completes,
	// they may leave normally.
	tableRec, err := e.App.FindRecordById("tables", tableID)
	if err != nil {
		return e.InternalServerError("load table", err)
	}
	currentHandID := tableRec.GetString("current_hand")
	if currentHandID != "" {
		if handRec, err := e.App.FindRecordById("hands", currentHandID); err == nil {
			if handRec.GetString("phase") != "complete" {
				inHand, _ := e.App.FindRecordsByFilter("hand_players",
					"hand = {:h} && user = {:u}", "", 1, 0,
					dbx.Params{"h": currentHandID, "u": e.Auth.Id})
				if len(inHand) > 0 {
					return e.BadRequestError(
						"cannot leave during an active hand; ask the dealer to fold you out", nil)
				}
			}
		}
	}

	// hand_players.seat is a Required, non-cascading relation, so PB
	// refuses to delete the seat while history rows reference it. Wipe
	// the leaving player's hand_players rows first. hands.actions and
	// hands.deck_state are preserved on the hand record itself, so
	// replay still works.
	err = e.App.RunInTransaction(func(tx core.App) error {
		hps, err := tx.FindRecordsByFilter("hand_players",
			"seat = {:s}", "", 0, 0, dbx.Params{"s": seat.Id})
		if err != nil {
			return fmt.Errorf("find hand_players: %w", err)
		}
		for _, hp := range hps {
			if err := tx.Delete(hp); err != nil {
				return fmt.Errorf("delete hand_player: %w", err)
			}
		}
		if err := tx.Delete(seat); err != nil {
			return fmt.Errorf("delete seat: %w", err)
		}
		return nil
	})
	if err != nil {
		return e.InternalServerError(err.Error(), err)
	}
	return e.JSON(200, map[string]any{"chips_returned": stack})
}

// ----- Add bot -----

type addBotRequest struct {
	SeatNumber  int    `json:"seat_number"`
	BuyInAmount int    `json:"buy_in_amount"`
	Personality string `json:"personality"`
}

// handleAddBot seats a new bot player at the given seat. Any
// authenticated user (typically a seated human at this table) can add
// a bot — there is no ownership check beyond auth, so anyone at the
// table can populate empty seats.
func handleAddBot(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")
	var req addBotRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if req.SeatNumber < 0 {
		return e.BadRequestError("seat_number must be non-negative", nil)
	}
	if _, ok := engine.Personalities[req.Personality]; !ok {
		return e.BadRequestError(
			fmt.Sprintf("unknown personality %q", req.Personality), nil)
	}

	tableRec, err := e.App.FindRecordById("tables", tableID)
	if err != nil {
		return e.NotFoundError("table not found", nil)
	}
	maxSeats := tableRec.GetInt("max_seats")
	if req.SeatNumber >= maxSeats {
		return e.BadRequestError(
			fmt.Sprintf("seat_number %d exceeds max_seats %d", req.SeatNumber, maxSeats), nil)
	}
	if req.BuyInAmount < tableRec.GetInt("buy_in") {
		return e.BadRequestError("buy_in_amount below table buy_in", nil)
	}

	// Reject if seat already occupied or if a hand is in progress
	// (introducing a player mid-hand would corrupt blinds/dealing).
	existing, _ := e.App.FindRecordsByFilter("seats",
		"table = {:t} && seat_number = {:s}", "", 1, 0,
		dbx.Params{"t": tableID, "s": req.SeatNumber})
	if len(existing) > 0 {
		return e.BadRequestError("seat already taken", nil)
	}
	if currentHandID := tableRec.GetString("current_hand"); currentHandID != "" {
		if hr, err := e.App.FindRecordById("hands", currentHandID); err == nil {
			if hr.GetString("phase") != "complete" {
				return e.BadRequestError("cannot add a bot during an active hand", nil)
			}
		}
	}

	col, err := e.App.FindCollectionByNameOrId("seats")
	if err != nil {
		return e.InternalServerError("seats collection missing", err)
	}
	rec := core.NewRecord(col)
	rec.Set("table", tableID)
	rec.Set("user", "")
	rec.Set("bot_personality", req.Personality)
	rec.Set("seat_number", req.SeatNumber)
	rec.Set("stack", req.BuyInAmount)
	rec.Set("status", "active")
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("save bot seat", err)
	}
	return e.JSON(200, map[string]any{
		"seat_id":     rec.Id,
		"personality": req.Personality,
	})
}

// ----- Remove bot -----

type removeBotRequest struct {
	SeatNumber int `json:"seat_number"`
}

// handleRemoveBot deletes a bot seat. The caller must be a seated
// human at the same table; the target seat must be a bot; no active
// hand may be in progress (mirrors handleLeave's mid-hand refusal).
func handleRemoveBot(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")
	var req removeBotRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid body", err)
	}

	callerSeats, err := e.App.FindRecordsByFilter("seats",
		"table = {:t} && user = {:u}", "", 1, 0,
		dbx.Params{"t": tableID, "u": e.Auth.Id})
	if err != nil {
		return e.InternalServerError("query seats", err)
	}
	if len(callerSeats) == 0 {
		return e.ForbiddenError("only a seated human at this table may remove a bot", nil)
	}

	target, _ := e.App.FindRecordsByFilter("seats",
		"table = {:t} && seat_number = {:s}", "", 1, 0,
		dbx.Params{"t": tableID, "s": req.SeatNumber})
	if len(target) == 0 {
		return e.NotFoundError("seat not found", nil)
	}
	bot := target[0]
	if bot.GetString("bot_personality") == "" {
		return e.BadRequestError("seat is not a bot", nil)
	}

	tableRec, err := e.App.FindRecordById("tables", tableID)
	if err != nil {
		return e.InternalServerError("load table", err)
	}
	if currentHandID := tableRec.GetString("current_hand"); currentHandID != "" {
		if hr, err := e.App.FindRecordById("hands", currentHandID); err == nil {
			if hr.GetString("phase") != "complete" {
				return e.BadRequestError(
					"cannot remove a bot during an active hand", nil)
			}
		}
	}

	// hand_players.seat is non-cascading; clear bot's history rows
	// before deleting the seat (same pattern as handleLeave).
	err = e.App.RunInTransaction(func(tx core.App) error {
		hps, err := tx.FindRecordsByFilter("hand_players",
			"seat = {:s}", "", 0, 0, dbx.Params{"s": bot.Id})
		if err != nil {
			return fmt.Errorf("find hand_players: %w", err)
		}
		for _, hp := range hps {
			if err := tx.Delete(hp); err != nil {
				return fmt.Errorf("delete hand_player: %w", err)
			}
		}
		if err := tx.Delete(bot); err != nil {
			return fmt.Errorf("delete bot seat: %w", err)
		}
		return nil
	})
	if err != nil {
		return e.InternalServerError(err.Error(), err)
	}
	return e.JSON(200, map[string]any{"seat_number": req.SeatNumber})
}

// ----- Ready for next hand -----

type readyRequest struct {
	Ready bool `json:"ready"`
}

func handleReady(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")
	var req readyRequest
	// Treat empty body as { ready: true } — it's the common case.
	_ = e.BindBody(&req)
	if e.Request.ContentLength == 0 {
		req.Ready = true
	}

	seats, err := e.App.FindRecordsByFilter("seats",
		"table = {:t} && user = {:u}", "", 1, 0,
		dbx.Params{"t": tableID, "u": e.Auth.Id})
	if err != nil {
		return e.InternalServerError("query seats", err)
	}
	if len(seats) == 0 {
		return e.NotFoundError("you are not seated at this table", nil)
	}
	seat := seats[0]
	seat.Set("ready_for_next", req.Ready)
	if err := e.App.Save(seat); err != nil {
		return e.InternalServerError("save ready state", err)
	}
	return e.JSON(200, map[string]any{"ready": req.Ready})
}

// ----- Start hand -----

type startHandRequest struct {
	VariantKey string `json:"variant_key"`
}

func handleStartHand(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")
	var req startHandRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	variant, err := engine.VariantByKey(req.VariantKey)
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	tableRec, err := e.App.FindRecordById("tables", tableID)
	if err != nil {
		return e.NotFoundError("table not found", nil)
	}
	sb := tableRec.GetInt("small_blind")
	bb := tableRec.GetInt("big_blind")

	seats, err := e.App.FindRecordsByFilter("seats",
		"table = {:t}", "seat_number", 0, 0, dbx.Params{"t": tableID})
	if err != nil {
		return e.InternalServerError("query seats", err)
	}
	if len(seats) < engine.MinPlayers {
		return e.BadRequestError(
			fmt.Sprintf("need at least %d seated players", engine.MinPlayers), nil)
	}
	if err := variant.CheckSeatFits(len(seats)); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	// Determine the dealer seat. We use tables.current_hand (set on
	// every start-hand, never cleared) as the "is there a hand to
	// rotate from" signal — same source the frontend uses to compute
	// who shows the start button. priorHands lookups are fragile
	// against stale data from older code paths that cleared
	// current_hand on completion.
	currentHandID := tableRec.GetString("current_hand")
	var prevHand *core.Record
	if currentHandID != "" {
		prevHand, _ = e.App.FindRecordById("hands", currentHandID)
	}
	dealerSeatNum := -1
	if prevHand == nil {
		// First hand at the table: lowest-numbered active *human* seat.
		// Bots are skipped from dealer rotation since they can't drive
		// start-hand themselves.
		for _, s := range seats {
			if s.GetString("status") != "active" {
				continue
			}
			if s.GetString("bot_personality") != "" {
				continue
			}
			dealerSeatNum = s.GetInt("seat_number")
			break
		}
		if dealerSeatNum < 0 {
			return e.InternalServerError("no active human seat to deal from", nil)
		}
	} else {
		// Subsequent hands require ready-up only from seats that were in
		// the previous hand and are still here. New sit-downs (and seats
		// whose hand_players were cleaned up via leave) skip the gate —
		// they have nothing to review.
		prevHandID := prevHand.Id
		notReady := []int{}
		for _, s := range seats {
			hps, _ := e.App.FindRecordsByFilter("hand_players",
				"hand = {:h} && seat = {:s}", "", 1, 0,
				dbx.Params{"h": prevHandID, "s": s.Id})
			if len(hps) == 0 {
				continue
			}
			if !s.GetBool("ready_for_next") {
				notReady = append(notReady, s.GetInt("seat_number"))
			}
		}
		if len(notReady) > 0 {
			return e.BadRequestError(
				fmt.Sprintf("waiting on seats %v to mark ready", notReady), nil)
		}

		prevDealer := prevHand.GetInt("dealer_seat")
		next, ok := nextActiveSeatClockwise(seats, prevDealer)
		if !ok {
			return e.InternalServerError("no active seat to deal to", nil)
		}
		dealerSeatNum = next
	}
	dealerCheck := false
	for _, s := range seats {
		if s.GetInt("seat_number") == dealerSeatNum && s.GetString("user") == e.Auth.Id {
			dealerCheck = true
			break
		}
	}
	if !dealerCheck {
		return e.ForbiddenError(
			fmt.Sprintf("only the dealer (seat %d) may start a hand", dealerSeatNum), nil)
	}

	// Build seated players input for the engine, sorted by seat number.
	players := make([]engine.SeatedPlayer, 0, len(seats))
	for _, s := range seats {
		players = append(players, engine.SeatedPlayer{
			Seat:  s.GetInt("seat_number"),
			Stack: s.GetInt("stack"),
		})
	}
	sort.Slice(players, func(i, j int) bool { return players[i].Seat < players[j].Seat })

	deck, err := engine.NewShuffledDeck()
	if err != nil {
		return e.InternalServerError("shuffle", err)
	}
	state, err := engine.Deal(variant, players, deck, sb, bb, dealerSeatNum)
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	// Persist hand + hand_players atomically.
	variantRec, err := e.App.FindFirstRecordByData("variants", "key", variant.Key)
	if err != nil {
		return e.InternalServerError("variant lookup", err)
	}

	var handID string
	err = e.App.RunInTransaction(func(tx core.App) error {
		handsCol, err := tx.FindCollectionByNameOrId("hands")
		if err != nil {
			return err
		}
		boardJSON, _ := json.Marshal(state.Board)
		actionsJSON, _ := json.Marshal(state.Actions)
		deckJSON, _ := json.Marshal(state.Deck)

		hRec := core.NewRecord(handsCol)
		hRec.Set("table", tableID)
		hRec.Set("variant", variantRec.Id)
		hRec.Set("dealer_seat", state.DealerSeat)
		hRec.Set("small_blind_seat", state.SmallBlindSeat)
		hRec.Set("big_blind_seat", state.BigBlindSeat)
		hRec.Set("community_cards", string(boardJSON))
		hRec.Set("pot", state.Pot)
		hRec.Set("phase", state.Phase.String())
		hRec.Set("current_actor_seat", state.CurrentActorSeat)
		hRec.Set("current_bet", state.CurrentBet)
		hRec.Set("actions", string(actionsJSON))
		hRec.Set("version", 0)
		hRec.Set("deck_state", string(deckJSON))
		if err := tx.Save(hRec); err != nil {
			return fmt.Errorf("save hand: %w", err)
		}
		handID = hRec.Id

		// One hand_players row per seat.
		hpCol, err := tx.FindCollectionByNameOrId("hand_players")
		if err != nil {
			return err
		}
		seatBySeatNumber := map[int]*core.Record{}
		for _, s := range seats {
			seatBySeatNumber[s.GetInt("seat_number")] = s
		}
		for _, ps := range state.Players {
			seatRec, ok := seatBySeatNumber[ps.Seat]
			if !ok {
				return fmt.Errorf("seat record missing for seat %d", ps.Seat)
			}
			holeJSON, _ := json.Marshal(ps.HoleCards)
			rec := core.NewRecord(hpCol)
			rec.Set("hand", handID)
			rec.Set("seat", seatRec.Id)
			rec.Set("user", seatRec.GetString("user"))
			rec.Set("hole_cards", string(holeJSON))
			rec.Set("status", string(ps.Status))
			if err := tx.Save(rec); err != nil {
				return fmt.Errorf("save hand_player: %w", err)
			}
		}

		// Reset ready_for_next on every seat — players must opt in to the
		// next hand after this one finishes.
		for _, s := range seats {
			if s.GetBool("ready_for_next") {
				s.Set("ready_for_next", false)
				if err := tx.Save(s); err != nil {
					return fmt.Errorf("reset ready: %w", err)
				}
			}
		}

		// Update tables.current_hand and current_dealer_seat.
		tableRec.Set("current_hand", handID)
		tableRec.Set("current_dealer_seat", dealerSeatNum)
		tableRec.Set("status", "active")
		if err := tx.Save(tableRec); err != nil {
			return fmt.Errorf("save table: %w", err)
		}
		return nil
	})
	if err != nil {
		return e.InternalServerError("start hand", err)
	}
	return e.JSON(200, map[string]any{"hand_id": handID})
}

// nextActiveSeatClockwise returns the seat_number of the next active
// *human* seat clockwise (numerically greater, wrapping) from `from`.
// Bots are excluded so the dealer button never lands on a seat that
// can't initiate the next start-hand call. Returns false if no
// eligible seats exist.
func nextActiveSeatClockwise(seats []*core.Record, from int) (int, bool) {
	type entry struct {
		num      int
		eligible bool
	}
	es := make([]entry, len(seats))
	for i, s := range seats {
		es[i] = entry{
			num: s.GetInt("seat_number"),
			eligible: s.GetString("status") == "active" &&
				s.GetString("bot_personality") == "",
		}
	}
	sort.Slice(es, func(i, j int) bool { return es[i].num < es[j].num })
	startIdx := -1
	for i, x := range es {
		if x.num == from {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		// `from` left the table; pick the first eligible seat.
		for _, x := range es {
			if x.eligible {
				return x.num, true
			}
		}
		return 0, false
	}
	for k := 1; k <= len(es); k++ {
		idx := (startIdx + k) % len(es)
		if es[idx].eligible {
			return es[idx].num, true
		}
	}
	return 0, false
}

// ----- Submit action -----

type actionRequest struct {
	ActionType string `json:"action_type"`
	Amount     int    `json:"amount"`
	Version    int    `json:"version"`
}

func handleAction(e *core.RequestEvent) error {
	handID := e.Request.PathValue("id")
	var req actionRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid body", err)
	}

	state, version, err := LoadHand(e.App, handID)
	if err != nil {
		if errors.Is(err, ErrHandNotFound) {
			return e.NotFoundError("hand not found", nil)
		}
		return e.InternalServerError("load hand", err)
	}

	// The seat's user must equal the caller.
	callerSeatNum := -1
	if err := e.App.RunInTransaction(func(tx core.App) error {
		hps, err := tx.FindRecordsByFilter("hand_players",
			"hand = {:h} && user = {:u}", "", 1, 0,
			dbx.Params{"h": handID, "u": e.Auth.Id})
		if err != nil {
			return err
		}
		if len(hps) == 0 {
			return errors.New("not a player at this hand")
		}
		seatRec, err := tx.FindRecordById("seats", hps[0].GetString("seat"))
		if err != nil {
			return err
		}
		callerSeatNum = seatRec.GetInt("seat_number")
		return nil
	}); err != nil {
		return e.ForbiddenError(err.Error(), nil)
	}
	if callerSeatNum != state.CurrentActorSeat {
		return e.ForbiddenError(
			fmt.Sprintf("not your turn (current_actor=%d, you=%d)",
				state.CurrentActorSeat, callerSeatNum), nil)
	}

	if req.Version != version {
		return apiConflict(e, fmt.Sprintf("version mismatch: server=%d, client=%d", version, req.Version))
	}

	newState, err := engine.ApplyAction(state, callerSeatNum, engine.ActionType(req.ActionType), req.Amount)
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	if err := SaveHand(e.App, handID, newState, version); err != nil {
		if errors.Is(err, ErrVersionMismatch) {
			return apiConflict(e, "version mismatch on save")
		}
		return e.InternalServerError("save hand", err)
	}

	// Note: tables.current_hand is intentionally NOT cleared on completion.
	// The completed hand stays referenced so clients can keep showing the
	// winner banner. The next start-hand call overwrites current_hand.

	return e.JSON(200, map[string]any{
		"phase":              newState.Phase.String(),
		"current_actor_seat": newState.CurrentActorSeat,
		"version":            version + 1,
	})
}

func apiConflict(e *core.RequestEvent, msg string) error {
	return e.JSON(409, map[string]any{"message": msg})
}

// ----- Fold player (dealer-only) -----

type foldPlayerRequest struct {
	SeatNumber int `json:"seat_number"`
}

func handleFoldPlayer(e *core.RequestEvent) error {
	handID := e.Request.PathValue("id")
	var req foldPlayerRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid body", err)
	}

	handRec, err := e.App.FindRecordById("hands", handID)
	if err != nil {
		return e.NotFoundError("hand not found", nil)
	}
	tableRec, err := e.App.FindRecordById("tables", handRec.GetString("table"))
	if err != nil {
		return e.InternalServerError("load table", err)
	}
	dealerSeatNum := tableRec.GetInt("current_dealer_seat")

	// Caller must be the dealer.
	dealerSeats, err := e.App.FindRecordsByFilter("seats",
		"table = {:t} && seat_number = {:s}", "", 1, 0,
		dbx.Params{"t": tableRec.Id, "s": dealerSeatNum})
	if err != nil || len(dealerSeats) == 0 {
		return e.InternalServerError("locate dealer seat", err)
	}
	if dealerSeats[0].GetString("user") != e.Auth.Id {
		return e.ForbiddenError("only the dealer may force-fold a player", nil)
	}

	state, version, err := LoadHand(e.App, handID)
	if err != nil {
		return e.InternalServerError("load hand", err)
	}
	if state.CurrentActorSeat != req.SeatNumber {
		return e.BadRequestError(
			fmt.Sprintf("seat %d is not the current actor (=%d)",
				req.SeatNumber, state.CurrentActorSeat), nil)
	}
	newState, err := engine.ApplyAction(state, req.SeatNumber, engine.ActionFold, 0)
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	if err := SaveHand(e.App, handID, newState, version); err != nil {
		if errors.Is(err, ErrVersionMismatch) {
			return apiConflict(e, "version mismatch")
		}
		return e.InternalServerError("save hand", err)
	}
	return e.JSON(200, map[string]any{"phase": newState.Phase.String()})
}

// ----- Delete table -----

// handleDeleteTable removes a table and all its dependent records.
// Only the table's creator may delete. PB's cascade chains
// (table → seats and table → hands → hand_players) race against the
// non-cascading hand_players.seat FK, so we wipe the graph in
// dependency order inside a single transaction instead of relying on
// PB's automatic cascade.
func handleDeleteTable(e *core.RequestEvent) error {
	tableID := e.Request.PathValue("id")

	tableRec, err := e.App.FindRecordById("tables", tableID)
	if err != nil {
		return e.NotFoundError("table not found", nil)
	}
	if tableRec.GetString("created_by") != e.Auth.Id {
		return e.ForbiddenError("only the table creator may delete it", nil)
	}

	err = e.App.RunInTransaction(func(tx core.App) error {
		hands, err := tx.FindRecordsByFilter("hands",
			"table = {:t}", "", 0, 0, dbx.Params{"t": tableID})
		if err != nil {
			return fmt.Errorf("find hands: %w", err)
		}
		for _, h := range hands {
			hps, err := tx.FindRecordsByFilter("hand_players",
				"hand = {:h}", "", 0, 0, dbx.Params{"h": h.Id})
			if err != nil {
				return fmt.Errorf("find hand_players: %w", err)
			}
			for _, hp := range hps {
				if err := tx.Delete(hp); err != nil {
					return fmt.Errorf("delete hand_player: %w", err)
				}
			}
			if err := tx.Delete(h); err != nil {
				return fmt.Errorf("delete hand: %w", err)
			}
		}

		seats, err := tx.FindRecordsByFilter("seats",
			"table = {:t}", "", 0, 0, dbx.Params{"t": tableID})
		if err != nil {
			return fmt.Errorf("find seats: %w", err)
		}
		for _, s := range seats {
			if err := tx.Delete(s); err != nil {
				return fmt.Errorf("delete seat: %w", err)
			}
		}

		if err := tx.Delete(tableRec); err != nil {
			return fmt.Errorf("delete table: %w", err)
		}
		return nil
	})
	if err != nil {
		return e.InternalServerError(err.Error(), err)
	}
	return e.JSON(200, map[string]any{"deleted": tableID})
}

// ----- Replay -----

func handleReplay(e *core.RequestEvent) error {
	handID := e.Request.PathValue("id")
	handRec, err := e.App.FindRecordById("hands", handID)
	if err != nil {
		return e.NotFoundError("hand not found", nil)
	}
	if handRec.GetString("phase") != "complete" {
		return e.BadRequestError("hand is not complete", nil)
	}

	var deck []engine.Card
	if raw := handRec.Get("deck_state"); raw != nil {
		_ = unmarshalAny(raw, &deck)
	}
	var actions []engine.Action
	if raw := handRec.Get("actions"); raw != nil {
		_ = unmarshalAny(raw, &actions)
	}
	return e.JSON(200, map[string]any{
		"deck_state": deck,
		"actions":    actions,
	})
}

// keep crand referenced in case future helpers use it directly.
var _ = crand.Reader
