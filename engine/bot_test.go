package engine

import (
	"math/rand"
	"testing"
)

// makeHoldemState builds a minimal Hold'em HandState for unit testing.
// `myHole` is the actor's hole cards; opponent hole cards are unset
// because Equity ignores them anyway. Players are seated 0..n-1 with
// `me` at seat 0.
func makeHoldemState(myHole []Card, board []Card, opponents int) HandState {
	players := make([]PlayerState, opponents+1)
	players[0] = PlayerState{Seat: 0, HoleCards: myHole, Stack: 1000, Status: PlayerActive}
	for i := 1; i <= opponents; i++ {
		players[i] = PlayerState{Seat: i, HoleCards: nil, Stack: 1000, Status: PlayerActive}
	}
	return HandState{
		VariantKey:       "holdem",
		Board:            board,
		Players:          players,
		Pot:              0,
		Phase:            PhasePreflop,
		CurrentActorSeat: 0,
		SmallBlind:       10,
		BigBlind:         20,
	}
}

func TestEquityPocketAcesPreflopBeatsTwoRandoms(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	state := makeHoldemState(MustParseCards("As Ah"), nil, 2)
	eq := Equity(state, 0, 1000, rng)
	if eq < 0.65 {
		t.Errorf("AA vs. 2 random in Hold'em: equity=%.3f, want > 0.65", eq)
	}
	if eq > 0.85 {
		t.Errorf("AA vs. 2 random in Hold'em: equity=%.3f suspiciously high", eq)
	}
}

func TestEquityWeakPairOnDangerBoardIsLow(t *testing.T) {
	// Post-river. Board: A-K-Q-J spades + 2c. My hole: 2h 7d (a pair of
	// 2s, no flush, no straight). Heads-up against a random opponent who
	// frequently has at least an ace.
	rng := rand.New(rand.NewSource(2))
	state := makeHoldemState(MustParseCards("2h 7d"), MustParseCards("As Ks Qs Js 2c"), 1)
	state.Phase = PhaseRiver
	eq := Equity(state, 0, 1000, rng)
	if eq > 0.35 {
		t.Errorf("pair of deuces on coordinated A-high board: equity=%.3f, want < 0.35", eq)
	}
}

func TestEquityFoldedOpponentsExcluded(t *testing.T) {
	// Add 2 opponents but mark one folded — equity should match a 1v1.
	rng := rand.New(rand.NewSource(3))
	state := makeHoldemState(MustParseCards("As Ah"), nil, 2)
	state.Players[2].Status = PlayerFolded
	eqHeads := Equity(state, 0, 800, rng)

	rng2 := rand.New(rand.NewSource(3))
	state2 := makeHoldemState(MustParseCards("As Ah"), nil, 1)
	eq1v1 := Equity(state2, 0, 800, rng2)

	// AA heads-up wins ~85%; we don't need the same RNG sequence to give
	// identical results, just both well above the 3-way figure.
	if eqHeads < 0.80 || eq1v1 < 0.80 {
		t.Errorf("AA heads-up should be >= 0.80; got folded-excluded=%.3f / 1v1=%.3f",
			eqHeads, eq1v1)
	}
}

func TestEquityNoLiveOpponentsReturnsOne(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	state := makeHoldemState(MustParseCards("2h 3d"), nil, 1)
	state.Players[1].Status = PlayerFolded
	eq := Equity(state, 0, 100, rng)
	if eq != 1.0 {
		t.Errorf("uncontested equity: got %.3f want 1.0", eq)
	}
}

// makeFacingPreflopRaise builds a heads-up Hold'em HandState where
// seat 0 is facing a 60-chip bet from seat 1, with no prior commits.
// toCall=60, pot=60, potOdds=0.5. Heads-up keeps the equity numbers in
// the trash/strong tests easy to reason about. Decide doesn't care how
// many seats are at the table — it just reads state.
func makeFacingPreflopRaise(myHole []Card) HandState {
	state := makeHoldemState(myHole, nil, 1)
	state.CurrentActorSeat = 0
	state.CurrentBet = 60
	state.Pot = 60
	state.Players[1].Stack = 940
	state.Actions = []Action{
		{Sequence: 1, Seat: 1, Phase: PhasePreflop, Type: ActionBet, Amount: 60},
	}
	return state
}

func TestDecideTightFoldsTrash(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	state := makeFacingPreflopRaise(MustParseCards("7d 2s"))
	tight := Personalities["tight"]
	// Run a handful of times — decision must consistently fold trash.
	folds := 0
	const trials = 20
	for i := 0; i < trials; i++ {
		action, _ := Decide(state, 0, tight, rng)
		if action == ActionFold {
			folds++
		}
	}
	if folds < 18 {
		t.Errorf("Tight Tina folded only %d/%d trash hands; want >= 18", folds, trials)
	}
}

func TestDecideStationDoesNotFoldTrashFreely(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	state := makeFacingPreflopRaise(MustParseCards("7d 2s"))
	station := Personalities["station"]
	folds := 0
	const trials = 20
	for i := 0; i < trials; i++ {
		action, _ := Decide(state, 0, station, rng)
		if action == ActionFold {
			folds++
		}
	}
	// Station's looseness pushes the equity floor down to 0.26; trash
	// equity 3-way is ~0.32, so they should mostly call. Allow up to
	// half folds to keep the test stable across RNG seeds, while still
	// distinguishing from Tight.
	if folds > 10 {
		t.Errorf("Calling Station folded %d/%d trash hands; want <= 10 (less foldy than tight)",
			folds, trials)
	}
}

func TestDecideStationNeverRaises(t *testing.T) {
	// Station has Aggression=0.1 and BluffFreq=0, so even on a strong
	// hand it should overwhelmingly call. This pins the personality.
	rng := rand.New(rand.NewSource(12))
	state := makeFacingPreflopRaise(MustParseCards("As Ah"))
	station := Personalities["station"]
	raises := 0
	const trials = 30
	for i := 0; i < trials; i++ {
		action, _ := Decide(state, 0, station, rng)
		if action == ActionRaise || action == ActionBet {
			raises++
		}
	}
	if raises > 6 {
		t.Errorf("Calling Station raised %d/%d times with AA; want <= 6", raises, trials)
	}
}

func TestDecideManiacRaisesStrongHands(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	state := makeFacingPreflopRaise(MustParseCards("As Ah"))
	maniac := Personalities["maniac"]
	raises := 0
	const trials = 30
	for i := 0; i < trials; i++ {
		action, _ := Decide(state, 0, maniac, rng)
		if action == ActionRaise {
			raises++
		}
	}
	if raises < 18 {
		t.Errorf("Maniac Mike raised only %d/%d times with AA; want >= 18", raises, trials)
	}
}

func TestDecideChecksForFreeWhenWeak(t *testing.T) {
	// toCall == 0, weak holding. Tight should always check, never fold
	// off a free option.
	rng := rand.New(rand.NewSource(14))
	state := makeHoldemState(MustParseCards("7d 2s"), MustParseCards("As Ks Qs Jc 9d"), 1)
	state.Phase = PhaseRiver
	state.CurrentBet = 0
	state.Pot = 100
	state.CurrentActorSeat = 0

	tight := Personalities["tight"]
	for i := 0; i < 20; i++ {
		action, _ := Decide(state, 0, tight, rng)
		if action == ActionFold {
			t.Errorf("tight folded a free check (iter %d): %s", i, action)
		}
	}
}

func TestDecideNeverEmitsIllegalAction(t *testing.T) {
	// Property check: Decide outputs must always be accepted by
	// ApplyAction. Run a few seeds across personalities.
	scenarios := []struct {
		name      string
		hole      []Card
		board     []Card
		phase     Phase
		curBet    int
		pot       int
		myStack   int
		actions   []Action
	}{
		{"preflop_unraised", MustParseCards("As Kh"), nil, PhasePreflop, 20, 30, 1000, []Action{
			{Sequence: 1, Seat: 1, Phase: PhasePreflop, Type: ActionPostBlind, Amount: 10},
			{Sequence: 2, Seat: 2, Phase: PhasePreflop, Type: ActionPostBlind, Amount: 20},
		}},
		{"flop_facing_bet", MustParseCards("As Kh"), MustParseCards("Ad 7c 2s 4d 8h"), PhaseFlop, 60, 200, 800, []Action{
			{Sequence: 1, Seat: 2, Phase: PhaseFlop, Type: ActionBet, Amount: 60},
		}},
		{"river_check_around", MustParseCards("Qd Qc"), MustParseCards("As Ks Qs Jc 9d"), PhaseRiver, 0, 200, 800, nil},
		{"short_stack", MustParseCards("9d 9c"), MustParseCards("As 7c 2s 4d 8h"), PhaseFlop, 200, 100, 50, []Action{
			{Sequence: 1, Seat: 2, Phase: PhaseFlop, Type: ActionBet, Amount: 200},
		}},
	}

	for _, sc := range scenarios {
		for pkey, p := range Personalities {
			for seed := int64(0); seed < 5; seed++ {
				rng := rand.New(rand.NewSource(seed))
				state := makeHoldemState(sc.hole, sc.board, 2)
				state.Phase = sc.phase
				state.CurrentBet = sc.curBet
				state.Pot = sc.pot
				state.Players[0].Stack = sc.myStack
				state.Actions = sc.actions

				action, amount := Decide(state, 0, p, rng)
				if _, err := ApplyAction(state, 0, action, amount); err != nil {
					t.Errorf("%s/%s/seed=%d: Decide returned (%s, %d) which ApplyAction rejected: %v",
						sc.name, pkey, seed, action, amount, err)
				}
			}
		}
	}
}
