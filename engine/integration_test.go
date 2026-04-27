package engine

import (
	"testing"
)

// dealForVariant scaffolds a 3-handed hand of the given variant and
// returns the initial state. Stacks 1000, blinds 10/20, dealer at seat 0,
// players at seats 0/2/5.
func dealForVariant(t *testing.T, variantKey string) HandState {
	t.Helper()
	v, err := VariantByKey(variantKey)
	if err != nil {
		t.Fatalf("variant %s: %v", variantKey, err)
	}
	deck := DeckFromSnapshot(scriptedDeck(""))
	state, err := Deal(v,
		[]SeatedPlayer{
			{Seat: 0, Stack: 1000},
			{Seat: 2, Stack: 1000},
			{Seat: 5, Stack: 1000},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal %s: %v", variantKey, err)
	}
	return state
}

// runCheckCallHand drives the given hand through preflop call/call/check
// then check around three more streets, asserting it reaches showdown
// cleanly and produces at least one winner.
func runCheckCallHand(t *testing.T, state HandState) HandState {
	t.Helper()
	steps := []struct {
		seat int
		typ  ActionType
	}{
		{0, ActionCall},  // dealer
		{2, ActionCall},  // SB
		{5, ActionCheck}, // BB option
		{2, ActionCheck},
		{5, ActionCheck},
		{0, ActionCheck},
		{2, ActionCheck},
		{5, ActionCheck},
		{0, ActionCheck},
		{2, ActionCheck},
		{5, ActionCheck},
		{0, ActionCheck},
	}
	var err error
	for i, s := range steps {
		state, err = ApplyAction(state, s.seat, s.typ, 0)
		if err != nil {
			t.Fatalf("step %d (seat %d %s): %v", i, s.seat, s.typ, err)
		}
	}
	if state.Phase != PhaseComplete {
		t.Fatalf("phase=%s want complete", state.Phase)
	}
	if len(state.Winners) == 0 {
		t.Fatalf("no winners")
	}
	return state
}

func TestEachVariantCompletesAFullHand(t *testing.T) {
	for _, v := range Variants {
		v := v
		t.Run(v.Key, func(t *testing.T) {
			state := dealForVariant(t, v.Key)
			// Every player got exactly hand_size hole cards.
			for _, p := range state.Players {
				if len(p.HoleCards) != v.HandSize {
					t.Fatalf("seat %d got %d hole cards want %d",
						p.Seat, len(p.HoleCards), v.HandSize)
				}
			}
			final := runCheckCallHand(t, state)

			// Pot is fully distributed.
			if final.Pot != 0 {
				t.Errorf("Pot=%d want 0", final.Pot)
			}
			// Conservation: total chips at table is 3 * 1000.
			total := 0
			for _, p := range final.Players {
				total += p.Stack
			}
			if total != 3000 {
				t.Errorf("total chips=%d want 3000", total)
			}
			// Winner has at least 5 chosen cards (or 0 if won uncontested,
			// which shouldn't happen here).
			for _, w := range final.Winners {
				if len(w.Cards) != 5 {
					t.Errorf("winner seat %d: %d chosen cards want 5",
						w.Seat, len(w.Cards))
				}
				if w.Class == "" {
					t.Errorf("winner seat %d: empty class", w.Seat)
				}
			}
		})
	}
}

// TestSidePotsDeferred documents that multi-way unequal-stack all-in
// settlement is intentionally not implemented in v1. When a real
// session hits this case, expand this test with the expected
// distribution and remove the t.Skip.
func TestSidePotsDeferred(t *testing.T) {
	t.Skip("side pots: TODO (v1.1)")
}
