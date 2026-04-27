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
	if err := e.App.Delete(seat); err != nil {
		return e.InternalServerError("delete seat", err)
	}
	return e.JSON(200, map[string]any{"chips_returned": stack})
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

	// Determine the dealer seat. For the first hand at a table, the
	// caller (must be seated) becomes the dealer. For subsequent hands,
	// rotate from the previous hand's dealer to the next active seat
	// clockwise; the caller must occupy that seat.
	priorHands, _ := e.App.FindRecordsByFilter("hands",
		"table = {:t}", "-created", 1, 0, dbx.Params{"t": tableID})
	dealerSeatNum := -1
	if len(priorHands) == 0 {
		for _, s := range seats {
			if s.GetString("user") == e.Auth.Id {
				dealerSeatNum = s.GetInt("seat_number")
				break
			}
		}
	} else {
		prevDealer := priorHands[0].GetInt("dealer_seat")
		next, ok := nextActiveSeatClockwise(seats, prevDealer)
		if !ok {
			return e.InternalServerError("no active seat to deal to", nil)
		}
		dealerSeatNum = next
	}
	if dealerSeatNum < 0 {
		return e.ForbiddenError("you must be seated to start a hand", nil)
	}
	dealerCheck := false
	for _, s := range seats {
		if s.GetInt("seat_number") == dealerSeatNum && s.GetString("user") == e.Auth.Id {
			dealerCheck = true
			break
		}
	}
	if !dealerCheck {
		return e.ForbiddenError("only the dealer may start a hand", nil)
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
// seat clockwise (numerically greater, wrapping) from `from`. Returns
// false if no active seats exist.
func nextActiveSeatClockwise(seats []*core.Record, from int) (int, bool) {
	type entry struct {
		num    int
		active bool
	}
	es := make([]entry, len(seats))
	for i, s := range seats {
		es[i] = entry{
			num:    s.GetInt("seat_number"),
			active: s.GetString("status") == "active",
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
		// `from` left the table; pick the first active seat.
		for _, x := range es {
			if x.active {
				return x.num, true
			}
		}
		return 0, false
	}
	for k := 1; k <= len(es); k++ {
		idx := (startIdx + k) % len(es)
		if es[idx].active {
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

	// If the hand reached completion, clear tables.current_hand so the
	// next start-hand begins fresh.
	if newState.Phase == engine.PhaseComplete {
		if hRec, err := e.App.FindRecordById("hands", handID); err == nil {
			tID := hRec.GetString("table")
			if tableRec, err := e.App.FindRecordById("tables", tID); err == nil {
				tableRec.Set("current_hand", "")
				_ = e.App.Save(tableRec)
			}
		}
	}

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
