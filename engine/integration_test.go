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

// TestSidePotsThreeWayUnequalAllIn covers the canonical side-pot case:
// a short stack all-in for less, two deeper stacks contesting an
// additional layer above the short stack's reach.
//
// Stacks 50 / 200 / 200, hole cards rigged so dealer (short) holds AA,
// SB holds KK, BB holds QQ on a blank board. Dealer wins the main pot
// outright; SB beats BB for the side pot.
func TestSidePotsThreeWayUnequalAllIn(t *testing.T) {
	// Card layout — Deal order is SB, BB, dealer for each round:
	//   card 0 -> SB(2), card 1 -> BB(5), card 2 -> dealer(0),
	//   card 3 -> SB(2), card 4 -> BB(5), card 5 -> dealer(0).
	// Then board cards 6..10.
	//   SB:     Kh Kd  (KK)
	//   BB:     Qh Qd  (QQ)
	//   Dealer: Ah Ad  (AA)
	//   Board:  2s 3h 4c 9d Tc  (no straights, no flushes)
	holdem, _ := VariantByKey("holdem")
	deck := DeckFromSnapshot(scriptedDeck("Kh Qh Ah Kd Qd Ad 2s 3h 4c 9d Tc"))
	state, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 50},
			{Seat: 2, Stack: 200},
			{Seat: 5, Stack: 200},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal: %v", err)
	}

	// Preflop: dealer all-in 50, SB raises to 100, BB calls 100.
	state, err = ApplyAction(state, 0, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("dealer all-in: %v", err)
	}
	state, err = ApplyAction(state, 2, ActionRaise, 90)
	if err != nil {
		t.Fatalf("SB raise: %v", err)
	}
	state, err = ApplyAction(state, 5, ActionCall, 0)
	if err != nil {
		t.Fatalf("BB call: %v", err)
	}

	// Flop, turn, river: SB and BB check it down. Dealer is all-in
	// (skipped). Each street has 2 active players so betting reopens.
	checks := []int{2, 5, 2, 5, 2, 5}
	for i, seat := range checks {
		state, err = ApplyAction(state, seat, ActionCheck, 0)
		if err != nil {
			t.Fatalf("postflop check %d (seat %d): %v", i, seat, err)
		}
	}

	if state.Phase != PhaseComplete {
		t.Fatalf("phase=%s want complete", state.Phase)
	}

	// Two pots: main 150 (all 3 eligible) and side 100 (seats 2 & 5).
	if len(state.Pots) != 2 {
		t.Fatalf("pots=%d want 2: %+v", len(state.Pots), state.Pots)
	}
	main := state.Pots[0]
	side := state.Pots[1]
	if main.Amount != 150 {
		t.Errorf("main amount=%d want 150", main.Amount)
	}
	if !equalInts(main.Eligible, []int{0, 2, 5}) {
		t.Errorf("main eligible=%v want [0 2 5]", main.Eligible)
	}
	if len(main.Winners) != 1 || main.Winners[0].Seat != 0 || main.Winners[0].Amount != 150 {
		t.Errorf("main winners=%+v want seat 0 amount 150", main.Winners)
	}
	if side.Amount != 100 {
		t.Errorf("side amount=%d want 100", side.Amount)
	}
	if !equalInts(side.Eligible, []int{2, 5}) {
		t.Errorf("side eligible=%v want [2 5]", side.Eligible)
	}
	if len(side.Winners) != 1 || side.Winners[0].Seat != 2 || side.Winners[0].Amount != 100 {
		t.Errorf("side winners=%+v want seat 2 amount 100", side.Winners)
	}

	// Final stacks: dealer 0+150, SB 100+100, BB 100+0.
	wantStacks := map[int]int{0: 150, 2: 200, 5: 100}
	for _, p := range state.Players {
		if p.Stack != wantStacks[p.Seat] {
			t.Errorf("seat %d stack=%d want %d", p.Seat, p.Stack, wantStacks[p.Seat])
		}
	}
	total := 0
	for _, p := range state.Players {
		total += p.Stack
	}
	if total != 450 {
		t.Errorf("conservation: total=%d want 450", total)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSidePotsUncalledBetAndFoldedContributor covers two side-pot
// edge cases at once: a folded player's blind seeds the main pot
// without earning eligibility, and the top layer has only one
// eligible seat (uncalled bet returned to the bettor).
//
// Stacks 200 / 50 / 200, dealer raises to 100 preflop, short SB
// shoves for 50, BB folds. With AA vs KK, SB wins the contested
// main pot and dealer reclaims the unmatched 50.
func TestSidePotsUncalledBetAndFoldedContributor(t *testing.T) {
	//   SB:     Ah Ad  (AA, wins main)
	//   BB:     7h 7d  (folds, contributes BB blind)
	//   Dealer: Kh Kd  (KK, only eligible for the side pot)
	//   Board:  2s 3h 4c 9d Tc
	holdem, _ := VariantByKey("holdem")
	deck := DeckFromSnapshot(scriptedDeck("Ah 7h Kh Ad 7d Kd 2s 3h 4c 9d Tc"))
	state, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 200},
			{Seat: 2, Stack: 50},
			{Seat: 5, Stack: 200},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal: %v", err)
	}

	state, err = ApplyAction(state, 0, ActionRaise, 100)
	if err != nil {
		t.Fatalf("dealer raise: %v", err)
	}
	state, err = ApplyAction(state, 2, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("SB all-in: %v", err)
	}
	state, err = ApplyAction(state, 5, ActionFold, 0)
	if err != nil {
		t.Fatalf("BB fold: %v", err)
	}

	// Only dealer is active and SB is all-in -> board runs out to showdown.
	if state.Phase != PhaseComplete {
		t.Fatalf("phase=%s want complete", state.Phase)
	}

	if len(state.Pots) != 2 {
		t.Fatalf("pots=%d want 2: %+v", len(state.Pots), state.Pots)
	}
	main := state.Pots[0]
	side := state.Pots[1]
	// Main pot: SB(50) + Dealer(50) + BB(20 blind) = 120, eligible 0 & 2.
	if main.Amount != 120 {
		t.Errorf("main amount=%d want 120", main.Amount)
	}
	if !equalInts(main.Eligible, []int{0, 2}) {
		t.Errorf("main eligible=%v want [0 2] (BB folded)", main.Eligible)
	}
	if len(main.Winners) != 1 || main.Winners[0].Seat != 2 || main.Winners[0].Amount != 120 {
		t.Errorf("main winners=%+v want seat 2 amount 120", main.Winners)
	}
	// Side pot: dealer's unmatched 50, only dealer eligible.
	if side.Amount != 50 {
		t.Errorf("side amount=%d want 50", side.Amount)
	}
	if !equalInts(side.Eligible, []int{0}) {
		t.Errorf("side eligible=%v want [0]", side.Eligible)
	}
	if len(side.Winners) != 1 || side.Winners[0].Seat != 0 || side.Winners[0].Amount != 50 {
		t.Errorf("side winners=%+v want seat 0 amount 50", side.Winners)
	}

	wantStacks := map[int]int{0: 150, 2: 120, 5: 180}
	for _, p := range state.Players {
		if p.Stack != wantStacks[p.Seat] {
			t.Errorf("seat %d stack=%d want %d", p.Seat, p.Stack, wantStacks[p.Seat])
		}
	}
}

// TestSidePotsTieSplitsSideEvenly exercises the tied-side-pot path:
// dealer wins the main pot outright, but the two deeper stacks have
// identical hands and split the side pot.
func TestSidePotsTieSplitsSideEvenly(t *testing.T) {
	//   SB:     Ks Kc  (KK)
	//   BB:     Kh Kd  (KK — same five-card hand as SB on this board)
	//   Dealer: Ah Ad  (AA, wins main outright)
	//   Board:  2s 3h 4c 9d Tc  (no flushes/straights for any hand)
	holdem, _ := VariantByKey("holdem")
	deck := DeckFromSnapshot(scriptedDeck("Ks Kh Ah Kc Kd Ad 2s 3h 4c 9d Tc"))
	state, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 50},
			{Seat: 2, Stack: 200},
			{Seat: 5, Stack: 200},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal: %v", err)
	}

	state, err = ApplyAction(state, 0, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("dealer all-in: %v", err)
	}
	state, err = ApplyAction(state, 2, ActionRaise, 90)
	if err != nil {
		t.Fatalf("SB raise: %v", err)
	}
	state, err = ApplyAction(state, 5, ActionCall, 0)
	if err != nil {
		t.Fatalf("BB call: %v", err)
	}
	for _, seat := range []int{2, 5, 2, 5, 2, 5} {
		state, err = ApplyAction(state, seat, ActionCheck, 0)
		if err != nil {
			t.Fatalf("check seat %d: %v", seat, err)
		}
	}

	if state.Phase != PhaseComplete {
		t.Fatalf("phase=%s want complete", state.Phase)
	}
	if len(state.Pots) != 2 {
		t.Fatalf("pots=%d want 2", len(state.Pots))
	}
	side := state.Pots[1]
	if side.Amount != 100 {
		t.Errorf("side amount=%d want 100", side.Amount)
	}
	if len(side.Winners) != 2 {
		t.Fatalf("side winners=%d want 2 (tie)", len(side.Winners))
	}
	// Even split, lowest seat first by convention.
	if side.Winners[0].Seat != 2 || side.Winners[0].Amount != 50 {
		t.Errorf("side winner[0]=%+v want seat 2 amount 50", side.Winners[0])
	}
	if side.Winners[1].Seat != 5 || side.Winners[1].Amount != 50 {
		t.Errorf("side winner[1]=%+v want seat 5 amount 50", side.Winners[1])
	}

	// Dealer wins all 150 of main; SB and BB each net 50 from the side pot.
	wantStacks := map[int]int{0: 150, 2: 150, 5: 150}
	for _, p := range state.Players {
		if p.Stack != wantStacks[p.Seat] {
			t.Errorf("seat %d stack=%d want %d", p.Seat, p.Stack, wantStacks[p.Seat])
		}
	}
}
