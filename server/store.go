package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"pocketpoker/engine"
)

// ErrVersionMismatch is returned by SaveHand when the persisted hand
// version no longer matches the caller's expected version, indicating
// a concurrent write. Callers should map this to HTTP 409.
var ErrVersionMismatch = errors.New("hand version mismatch")

// ErrHandNotFound is returned by LoadHand when the hand id doesn't exist.
var ErrHandNotFound = errors.New("hand not found")

// LoadHand reads the persisted hand state from PocketBase and rebuilds
// the engine.HandState. The returned version is the value as of read;
// callers must echo it back into SaveHand for optimistic concurrency.
func LoadHand(app core.App, handID string) (engine.HandState, int, error) {
	handRec, err := app.FindRecordById("hands", handID)
	if err != nil {
		return engine.HandState{}, 0, fmt.Errorf("%w: %v", ErrHandNotFound, err)
	}

	variantRec, err := app.FindRecordById("variants", handRec.GetString("variant"))
	if err != nil {
		return engine.HandState{}, 0, fmt.Errorf("load variant: %w", err)
	}
	tableRec, err := app.FindRecordById("tables", handRec.GetString("table"))
	if err != nil {
		return engine.HandState{}, 0, fmt.Errorf("load table: %w", err)
	}

	playerRecs, err := app.FindRecordsByFilter("hand_players",
		"hand = {:hand}", "", 0, 0, dbx.Params{"hand": handID})
	if err != nil {
		return engine.HandState{}, 0, fmt.Errorf("load hand_players: %w", err)
	}

	// We need the action log to derive per-seat in-hand commits below.
	var actions []engine.Action
	if raw := handRec.Get("actions"); raw != nil {
		if err := unmarshalAny(raw, &actions); err != nil {
			return engine.HandState{}, 0, fmt.Errorf("decode actions: %w", err)
		}
	}
	committedBySeat := map[int]int{}
	for _, a := range actions {
		committedBySeat[a.Seat] += a.Amount
	}

	players := make([]engine.PlayerState, 0, len(playerRecs))
	for _, pr := range playerRecs {
		seatRec, err := app.FindRecordById("seats", pr.GetString("seat"))
		if err != nil {
			return engine.HandState{}, 0, fmt.Errorf("load seat: %w", err)
		}
		var hole []engine.Card
		if raw := pr.Get("hole_cards"); raw != nil {
			if err := unmarshalAny(raw, &hole); err != nil {
				return engine.HandState{}, 0, fmt.Errorf("decode hole_cards: %w", err)
			}
		}
		seatNum := seatRec.GetInt("seat_number")
		// seats.stack is only persisted at hand start and hand completion.
		// Mid-hand, derive the running stack as start_of_hand - committed.
		stack := seatRec.GetInt("stack") - committedBySeat[seatNum]
		players = append(players, engine.PlayerState{
			Seat:      seatNum,
			Stack:     stack,
			Status:    engine.PlayerStatus(pr.GetString("status")),
			HoleCards: hole,
		})
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].Seat < players[j].Seat
	})

	var board []engine.Card
	if raw := handRec.Get("community_cards"); raw != nil {
		if err := unmarshalAny(raw, &board); err != nil {
			return engine.HandState{}, 0, fmt.Errorf("decode community_cards: %w", err)
		}
	}
	var deck []engine.Card
	if raw := handRec.Get("deck_state"); raw != nil {
		if err := unmarshalAny(raw, &deck); err != nil {
			return engine.HandState{}, 0, fmt.Errorf("decode deck_state: %w", err)
		}
	}
	var winners []engine.SeatResult
	if raw := handRec.Get("winner_seats"); raw != nil {
		_ = unmarshalAny(raw, &winners) // optional
	}

	variantKey := variantRec.GetString("key")
	v, err := engine.VariantByKey(variantKey)
	if err != nil {
		return engine.HandState{}, 0, fmt.Errorf("unknown variant %q: %w", variantKey, err)
	}

	state := engine.HandState{
		VariantKey:       variantKey,
		Deck:             deck,
		DeckPos:          v.HandSize*len(players) + len(board),
		Board:            board,
		Players:          players,
		Pot:              handRec.GetInt("pot"),
		Phase:            phaseFromString(handRec.GetString("phase")),
		DealerSeat:       handRec.GetInt("dealer_seat"),
		SmallBlindSeat:   handRec.GetInt("small_blind_seat"),
		BigBlindSeat:     handRec.GetInt("big_blind_seat"),
		CurrentActorSeat: handRec.GetInt("current_actor_seat"),
		CurrentBet:       handRec.GetInt("current_bet"),
		SmallBlind:       tableRec.GetInt("small_blind"),
		BigBlind:         tableRec.GetInt("big_blind"),
		Actions:          actions,
		Winners:          winners,
	}

	return state, handRec.GetInt("version"), nil
}

// SaveHand writes the new state back to PB inside a transaction with
// an optimistic-concurrency check on the hand's `version` column.
// The hand_players status fields are also updated; player stacks are
// only persisted to the seats collection when the hand is complete.
func SaveHand(app core.App, handID string, state engine.HandState, expectedVersion int) error {
	return app.RunInTransaction(func(tx core.App) error {
		handRec, err := tx.FindRecordById("hands", handID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrHandNotFound, err)
		}
		if handRec.GetInt("version") != expectedVersion {
			return ErrVersionMismatch
		}

		boardJSON, err := json.Marshal(state.Board)
		if err != nil {
			return err
		}
		actionsJSON, err := json.Marshal(state.Actions)
		if err != nil {
			return err
		}
		var winnersJSON []byte
		if len(state.Winners) > 0 {
			winnersJSON, err = json.Marshal(state.Winners)
			if err != nil {
				return err
			}
		}

		handRec.Set("community_cards", string(boardJSON))
		handRec.Set("pot", state.Pot)
		handRec.Set("phase", state.Phase.String())
		handRec.Set("current_actor_seat", state.CurrentActorSeat)
		handRec.Set("current_bet", state.CurrentBet)
		handRec.Set("actions", string(actionsJSON))
		handRec.Set("version", expectedVersion+1)
		if winnersJSON != nil {
			handRec.Set("winner_seats", string(winnersJSON))
		}
		if err := tx.Save(handRec); err != nil {
			return fmt.Errorf("save hand: %w", err)
		}

		// Update each hand_player's status if it changed.
		playerRecs, err := tx.FindRecordsByFilter("hand_players",
			"hand = {:hand}", "", 0, 0, dbx.Params{"hand": handID})
		if err != nil {
			return fmt.Errorf("load hand_players: %w", err)
		}
		seatToStatus := map[int]engine.PlayerStatus{}
		seatToStack := map[int]int{}
		for _, p := range state.Players {
			seatToStatus[p.Seat] = p.Status
			seatToStack[p.Seat] = p.Stack
		}
		for _, pr := range playerRecs {
			seatRec, err := tx.FindRecordById("seats", pr.GetString("seat"))
			if err != nil {
				return err
			}
			seatNum := seatRec.GetInt("seat_number")
			newStatus, ok := seatToStatus[seatNum]
			if !ok {
				continue
			}
			if string(newStatus) != pr.GetString("status") {
				pr.Set("status", string(newStatus))
				if err := tx.Save(pr); err != nil {
					return fmt.Errorf("save hand_player: %w", err)
				}
			}
		}

		// At hand completion, propagate updated stacks to the seats
		// collection so the next hand sees correct chip counts.
		if state.Phase == engine.PhaseComplete {
			for _, pr := range playerRecs {
				seatRec, err := tx.FindRecordById("seats", pr.GetString("seat"))
				if err != nil {
					return err
				}
				seatNum := seatRec.GetInt("seat_number")
				if newStack, ok := seatToStack[seatNum]; ok {
					seatRec.Set("stack", newStack)
					if err := tx.Save(seatRec); err != nil {
						return fmt.Errorf("save seat stack: %w", err)
					}
				}
			}
		}
		return nil
	})
}

// unmarshalAny coerces a value retrieved via Record.Get into v.
// PB sometimes returns a string and sometimes a []byte for JSON columns;
// handle both. Other shapes are decoded by re-marshalling.
func unmarshalAny(raw any, v any) error {
	switch x := raw.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return nil
		}
		return json.Unmarshal([]byte(x), v)
	case []byte:
		if len(x) == 0 {
			return nil
		}
		return json.Unmarshal(x, v)
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, v)
	}
}

// phaseFromString maps the persisted phase label back to engine.Phase.
func phaseFromString(s string) engine.Phase {
	switch s {
	case "preflop":
		return engine.PhasePreflop
	case "flop":
		return engine.PhaseFlop
	case "turn":
		return engine.PhaseTurn
	case "river":
		return engine.PhaseRiver
	case "showdown":
		return engine.PhaseShowdown
	case "complete":
		return engine.PhaseComplete
	}
	return engine.PhasePreflop
}
