package engine

import (
	"strings"
	"testing"
)

// dealStandard sets up a 3-handed Hold'em hand at seats 0, 2, 5 with
// 1000 chips each, dealer at seat 0, SB=10, BB=20. The deck snapshot
// must be at least 11 cards for a full hand to play out.
func dealStandard(t *testing.T, deckCards []Card) HandState {
	t.Helper()
	holdem, err := VariantByKey("holdem")
	if err != nil {
		t.Fatalf("variant: %v", err)
	}
	deck := DeckFromSnapshot(deckCards)
	state, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 1000},
			{Seat: 2, Stack: 1000},
			{Seat: 5, Stack: 1000},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal: %v", err)
	}
	return state
}

// scriptedDeck returns a 52-card deck composed of the supplied prefix
// followed by the canonical-order remainder, with no overlap.
func scriptedDeck(prefix string) []Card {
	pre := MustParseCards(prefix)
	used := map[Card]bool{}
	for _, c := range pre {
		used[c] = true
	}
	out := make([]Card, 0, DeckSize)
	out = append(out, pre...)
	for s := Clubs; s <= Spades; s++ {
		for r := Two; r <= Ace; r++ {
			c := Card{Rank: r, Suit: s}
			if !used[c] {
				out = append(out, c)
			}
		}
	}
	if len(out) != DeckSize {
		panic("scriptedDeck did not produce 52 cards")
	}
	return out
}

func TestDealPostsBlindsAndSetsActor(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	if state.SmallBlindSeat != 2 {
		t.Errorf("SBSeat=%d want 2", state.SmallBlindSeat)
	}
	if state.BigBlindSeat != 5 {
		t.Errorf("BBSeat=%d want 5", state.BigBlindSeat)
	}
	if state.CurrentActorSeat != 0 {
		t.Errorf("first actor=%d want 0 (dealer in 3-handed)", state.CurrentActorSeat)
	}
	if state.CurrentBet != 20 {
		t.Errorf("CurrentBet=%d want 20", state.CurrentBet)
	}
	if state.Pot != 30 {
		t.Errorf("Pot=%d want 30 (sb+bb)", state.Pot)
	}
	if len(state.Actions) != 2 {
		t.Errorf("Actions=%d want 2 blinds", len(state.Actions))
	}
	for _, p := range state.Players {
		if len(p.HoleCards) != 2 {
			t.Errorf("seat %d got %d hole cards want 2", p.Seat, len(p.HoleCards))
		}
	}
}

func TestDealRequiresMinPlayers(t *testing.T) {
	holdem, _ := VariantByKey("holdem")
	deck := DeckFromSnapshot(scriptedDeck(""))
	_, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 1000},
			{Seat: 1, Stack: 1000},
		},
		deck, 10, 20, 0)
	if err == nil {
		t.Error("expected error for 2-player Deal (min is 3)")
	}
}

func TestDealRejectsTooManySeatsForVariant(t *testing.T) {
	dubai, _ := VariantByKey("dubai")
	players := make([]SeatedPlayer, 7)
	for i := range players {
		players[i] = SeatedPlayer{Seat: i, Stack: 1000}
	}
	deck := DeckFromSnapshot(scriptedDeck(""))
	_, err := Deal(dubai, players, deck, 10, 20, 0)
	if err == nil {
		t.Error("expected error for 7 seats in dubai (max 6)")
	}
}

func TestApplyActionRejectsWrongSeat(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	// state.CurrentActorSeat == 0; try seat 2.
	_, err := ApplyAction(state, 2, ActionCall, 0)
	if err == nil {
		t.Error("expected error: not seat 2's turn")
	}
}

func TestApplyActionFoldRemovesPlayer(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	next, err := ApplyAction(state, 0, ActionFold, 0)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	idx, _ := next.playerIndex(0)
	if next.Players[idx].Status != PlayerFolded {
		t.Errorf("status=%s want folded", next.Players[idx].Status)
	}
	if next.CurrentActorSeat != 2 {
		t.Errorf("next actor=%d want 2", next.CurrentActorSeat)
	}
}

func TestCannotCheckWhenBetOutstanding(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	// Seat 0 must call/raise/fold; cannot check.
	_, err := ApplyAction(state, 0, ActionCheck, 0)
	if err == nil {
		t.Error("expected error: cannot check with bet outstanding")
	}
}

func TestCannotBetWhenCurrentBetExists(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	// CurrentBet=20 (BB). Bet should fail; must use Raise.
	_, err := ApplyAction(state, 0, ActionBet, 50)
	if err == nil {
		t.Error("expected error: cannot bet when current_bet>0")
	}
}

func TestRaiseBelowMinIsRejected(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	// CurrentBet=20, BB=20. Min raise total = 40. Try raising to 30.
	_, err := ApplyAction(state, 0, ActionRaise, 30) // amount=30 from 0 committed -> total 30
	if err == nil {
		t.Error("expected error: raise total below min")
	}
}

func TestRaiseExactlyMinIsAccepted(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	// Min raise total = 40 (current_bet 20 + bb 20). Seat 0 has committed 0.
	next, err := ApplyAction(state, 0, ActionRaise, 40)
	if err != nil {
		t.Fatalf("min raise rejected: %v", err)
	}
	if next.CurrentBet != 40 {
		t.Errorf("CurrentBet=%d want 40", next.CurrentBet)
	}
}

func TestWonByFoldEndsHand(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	state, err := ApplyAction(state, 0, ActionFold, 0)
	if err != nil {
		t.Fatalf("seat 0 fold: %v", err)
	}
	state, err = ApplyAction(state, 2, ActionFold, 0)
	if err != nil {
		t.Fatalf("seat 2 fold: %v", err)
	}
	if state.Phase != PhaseComplete {
		t.Errorf("Phase=%s want complete", state.Phase)
	}
	if len(state.Winners) != 1 || state.Winners[0].Seat != 5 {
		t.Errorf("winners=%v want [seat 5]", state.Winners)
	}
	// BB(seat 5) started with 1000, posted 20, won pot=30 → ends with 1010.
	idx, _ := state.playerIndex(5)
	if state.Players[idx].Stack != 1010 {
		t.Errorf("BB stack=%d want 1010", state.Players[idx].Stack)
	}
}

func TestRoundCompleteWhenAllCallAndBBChecks(t *testing.T) {
	state := dealStandard(t, scriptedDeck(""))
	var err error
	state, err = ApplyAction(state, 0, ActionCall, 0)
	if err != nil {
		t.Fatalf("seat 0 call: %v", err)
	}
	state, err = ApplyAction(state, 2, ActionCall, 0)
	if err != nil {
		t.Fatalf("seat 2 call: %v", err)
	}
	// At this point all 3 have committed 20 but BB has not voluntarily acted.
	if state.Phase != PhasePreflop {
		t.Errorf("phase advanced too early to %s", state.Phase)
	}
	if state.CurrentActorSeat != 5 {
		t.Errorf("actor=%d want 5 (BB option)", state.CurrentActorSeat)
	}
	state, err = ApplyAction(state, 5, ActionCheck, 0)
	if err != nil {
		t.Fatalf("BB check: %v", err)
	}
	if state.Phase != PhaseFlop {
		t.Errorf("phase=%s want flop after BB check", state.Phase)
	}
	if len(state.Board) != 3 {
		t.Errorf("Board=%d cards want 3", len(state.Board))
	}
	// Postflop first actor = SB (seat 2).
	if state.CurrentActorSeat != 2 {
		t.Errorf("postflop actor=%d want 2", state.CurrentActorSeat)
	}
	if state.CurrentBet != 0 {
		t.Errorf("CurrentBet=%d want 0 postflop", state.CurrentBet)
	}
}

func TestFullHoldemHandAllCheckCall_SBWinsHighCard(t *testing.T) {
	// Hole-cards by seat (sorted: 2, 5, 0; dealer=0):
	//   SB(2): card 0, 3
	//   BB(5): card 1, 4
	//   dealer(0): card 2, 5
	// We choose:
	//   0: Ah, 1: Ad, 2: Ks, 3: Kh, 4: Qc, 5: Qd
	//   board (6..10): 2s 3h 4c 9d Tc
	// Ranks:
	//   SB: AhKh -> A-high, K kicker
	//   BB: AdQc -> A-high, Q kicker (worse)
	//   Dealer: KsQd -> K-high
	// SB wins.
	deck := scriptedDeck("Ah Ad Ks Kh Qc Qd 2s 3h 4c 9d Tc")
	state := dealStandard(t, deck)

	steps := []struct {
		seat int
		typ  ActionType
	}{
		{0, ActionCall},  // dealer
		{2, ActionCall},  // SB
		{5, ActionCheck}, // BB option
		// flop
		{2, ActionCheck},
		{5, ActionCheck},
		{0, ActionCheck},
		// turn
		{2, ActionCheck},
		{5, ActionCheck},
		{0, ActionCheck},
		// river
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
	if len(state.Winners) != 1 {
		t.Fatalf("winners=%d want 1", len(state.Winners))
	}
	if state.Winners[0].Seat != 2 {
		t.Errorf("winner=%d want seat 2 (SB AKh)", state.Winners[0].Seat)
	}
	// Pot = 60 chips total. Each seat contributed 20.
	// SB starting 1000, posted 10, called 10 -> -20, then wins 60 -> +40 net.
	idx, _ := state.playerIndex(2)
	if state.Players[idx].Stack != 1040 {
		t.Errorf("SB stack=%d want 1040", state.Players[idx].Stack)
	}
}

func TestFullHandRaiseAndFold(t *testing.T) {
	// Same setup; dealer raises pot, SB calls, BB folds.
	deck := scriptedDeck("Ah Ad Ks Kh Qc Qd 2s 3h 4c 9d Tc")
	state := dealStandard(t, deck)

	var err error
	state, err = ApplyAction(state, 0, ActionRaise, 60) // raise to 60 total
	if err != nil {
		t.Fatalf("raise: %v", err)
	}
	if state.CurrentBet != 60 {
		t.Errorf("CurrentBet=%d want 60", state.CurrentBet)
	}
	state, err = ApplyAction(state, 2, ActionCall, 0)
	if err != nil {
		t.Fatalf("SB call: %v", err)
	}
	state, err = ApplyAction(state, 5, ActionFold, 0)
	if err != nil {
		t.Fatalf("BB fold: %v", err)
	}
	// Now action returns to dealer who has already acted at the current bet,
	// SB has acted, BB folded. Round should be complete -> flop.
	if state.Phase != PhaseFlop {
		t.Errorf("phase=%s want flop", state.Phase)
	}
	// Pot: SB posted 10, called 60 (50 more) = 60; dealer raised to 60; BB posted 20.
	// Pot = 60 + 60 + 20 = 140.
	if state.Pot != 140 {
		t.Errorf("Pot=%d want 140", state.Pot)
	}
	if state.CurrentActorSeat != 2 {
		t.Errorf("postflop actor=%d want 2 (SB first)", state.CurrentActorSeat)
	}
}

func TestEqualStackAllInsRunOutBoardAndSettle(t *testing.T) {
	// 3 players each go all-in for the same total -> no side pots needed.
	holdem, _ := VariantByKey("holdem")
	deck := DeckFromSnapshot(scriptedDeck("Ah Ad Ks Kh Qc Qd 2s 3h 4c 9d Tc"))
	state, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 100},
			{Seat: 2, Stack: 100},
			{Seat: 5, Stack: 100},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal: %v", err)
	}

	// Dealer goes all-in for 100 (raise to 100 from 0 committed).
	state, err = ApplyAction(state, 0, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("dealer all-in: %v", err)
	}
	// SB calls all-in (already 10 in, needs 90 more, has 90 left).
	state, err = ApplyAction(state, 2, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("SB all-in: %v", err)
	}
	// BB calls all-in (already 20 in, needs 80 more, has 80 left).
	state, err = ApplyAction(state, 5, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("BB all-in: %v", err)
	}

	// All three are all-in for 100 each. The engine should run out the
	// board and reach showdown, declaring a winner.
	if state.Phase != PhaseComplete {
		t.Errorf("phase=%s want complete", state.Phase)
	}
	if len(state.Winners) == 0 {
		t.Fatalf("no winners")
	}
	if state.Pot != 0 {
		t.Errorf("Pot=%d want 0 after distribution", state.Pot)
	}
	totalChips := 0
	for _, p := range state.Players {
		totalChips += p.Stack
	}
	if totalChips != 300 {
		t.Errorf("total chips=%d want 300", totalChips)
	}
}

func TestUnequalStackAllInsTriggerSidePotError(t *testing.T) {
	// A short-stacked dealer goes all-in for less than the others'
	// commitment -> side pots required, v1 should error at showdown.
	holdem, _ := VariantByKey("holdem")
	deck := DeckFromSnapshot(scriptedDeck(""))
	state, err := Deal(holdem,
		[]SeatedPlayer{
			{Seat: 0, Stack: 50}, // shortest
			{Seat: 2, Stack: 200},
			{Seat: 5, Stack: 200},
		},
		deck, 10, 20, 0)
	if err != nil {
		t.Fatalf("Deal: %v", err)
	}
	// Dealer all-in for 50.
	state, err = ApplyAction(state, 0, ActionAllIn, 0)
	if err != nil {
		t.Fatalf("dealer all-in: %v", err)
	}
	// SB raises to 100 (calling dealer's 50 plus extra). SB has 200-10=190 left.
	state, err = ApplyAction(state, 2, ActionRaise, 90) // total 100
	if err != nil {
		t.Fatalf("SB raise: %v", err)
	}
	// BB calls.
	state, err = ApplyAction(state, 5, ActionCall, 0)
	if err == nil && state.Phase == PhaseComplete && state.Winners != nil {
		t.Fatalf("expected side-pot error at showdown, got clean settlement")
	}
	if err != nil && !strings.Contains(err.Error(), "side pot") {
		// Also acceptable if the error surfaces at showdown.
		t.Fatalf("unexpected error: %v", err)
	}
}
