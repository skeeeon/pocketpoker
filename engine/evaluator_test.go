package engine

import (
	"testing"

	cpoker "github.com/chehsunliu/poker"
)

// rankClass returns the chehsunliu rank class for a HandRank.
func rankClass(r HandRank) string {
	return cpoker.RankString(int32(r))
}

func TestCombinations(t *testing.T) {
	cards := MustParseCards("As Kc Qh")
	cases := []struct {
		k    int
		want int
	}{
		{0, 1}, // single empty combo
		{1, 3},
		{2, 3},
		{3, 1},
		{4, 0}, // out of range -> nil
	}
	for _, tc := range cases {
		got := combinations(cards, tc.k)
		if len(got) != tc.want {
			t.Errorf("combinations(%d): got %d combos want %d", tc.k, len(got), tc.want)
		}
	}
}

func TestEvaluateAllHandCategories(t *testing.T) {
	// Every category, evaluated as a 5-card hand.
	cases := []struct {
		name  string
		cards string
		class string
	}{
		{"royal flush", "As Ks Qs Js Ts", "Straight Flush"},
		{"straight flush", "9h 8h 7h 6h 5h", "Straight Flush"},
		{"four of a kind", "Ad As Ac Ah Kh", "Four of a Kind"},
		{"full house", "Kd Kc Ks 5h 5d", "Full House"},
		{"flush", "Ad Jd 9d 6d 3d", "Flush"},
		{"straight", "9c 8d 7h 6s 5d", "Straight"},
		{"wheel straight", "5c 4d 3h 2s Ad", "Straight"},
		{"three of a kind", "Qd Qc Qs 7h 4d", "Three of a Kind"},
		{"two pair", "Jd Jc 8s 8h 2d", "Two Pair"},
		{"pair", "Td Tc 9s 5h 2d", "Pair"},
		{"high card", "Ad Jd 9c 6h 3s", "High Card"},
	}
	for _, tc := range cases {
		cards := MustParseCards(tc.cards)
		r := evaluateFive(cards)
		if got := rankClass(r); got != tc.class {
			t.Errorf("%s: rank=%d class=%q want %q", tc.name, r, got, tc.class)
		}
	}
}

func TestEvaluateOrdering(t *testing.T) {
	// Stronger hand has strictly lower rank.
	straightFlush := evaluateFive(MustParseCards("9h 8h 7h 6h 5h"))
	straight := evaluateFive(MustParseCards("9c 8d 7h 6s 5d"))
	pair := evaluateFive(MustParseCards("Td Tc 9s 5h 2d"))
	highCard := evaluateFive(MustParseCards("Ad Jd 9c 6h 3s"))

	if !(straightFlush < straight && straight < pair && pair < highCard) {
		t.Errorf("ordering broken: SF=%d S=%d P=%d HC=%d",
			straightFlush, straight, pair, highCard)
	}
}

func TestBestHandHoldemRoyalFlush(t *testing.T) {
	holdem, _ := VariantByKey("holdem")
	hole := MustParseCards("As Ks")
	board := MustParseCards("Qs Js Ts 2c 3d")

	res, err := BestHand(hole, board, holdem)
	if err != nil {
		t.Fatalf("BestHand: %v", err)
	}
	if res.Class != "Straight Flush" {
		t.Errorf("class=%q want Straight Flush", res.Class)
	}
	if res.Rank != 1 {
		t.Errorf("rank=%d want 1 (royal)", res.Rank)
	}
	if len(res.Cards) != 5 {
		t.Errorf("got %d cards want 5", len(res.Cards))
	}
}

func TestBestHandHoldemWheel(t *testing.T) {
	holdem, _ := VariantByKey("holdem")
	hole := MustParseCards("As 2c")
	board := MustParseCards("3d 4h 5s Kc Qh")

	res, err := BestHand(hole, board, holdem)
	if err != nil {
		t.Fatalf("BestHand: %v", err)
	}
	if res.Class != "Straight" {
		t.Errorf("class=%q want Straight (A-2-3-4-5)", res.Class)
	}
}

func TestOmahaStrictTwoFromHandSuppressesHoldemRoyal(t *testing.T) {
	// Identical cards as TestBestHandHoldemRoyalFlush would be, but with
	// 4 hole cards. Hold'em-style play would yield a royal flush; Omaha
	// rules force exactly 2 from hand and 3 from board, which on this
	// board only yields a Q-high straight.
	omaha, _ := VariantByKey("omaha")
	hole := MustParseCards("As Ks Qs Js")     // 4 spades in hand
	board := MustParseCards("Ts 9c 8d 7h 2c") // only one spade on board

	resOmaha, err := BestHand(hole, board, omaha)
	if err != nil {
		t.Fatalf("Omaha BestHand: %v", err)
	}
	if resOmaha.Class != "Straight" {
		t.Errorf("Omaha class=%q want Straight (Q-high)", resOmaha.Class)
	}

	// Sanity: rank Q-high straight is well above royal-flush rank 1.
	if resOmaha.Rank <= 1 {
		t.Errorf("Omaha rank=%d should not be at royal-flush level", resOmaha.Rank)
	}
}

func TestNovaScotiaCapsHandUsageAtTwo(t *testing.T) {
	// Hand contains a royal flush in spades, but Nova Scotia limits to
	// at most 2 cards from hand. With this board the best result must
	// be far weaker than royal flush.
	ns, _ := VariantByKey("nova_scotia")
	hole := MustParseCards("As Ks Qs Js Ts 9c 8d")
	board := MustParseCards("2c 3d 4h 5s 7h")

	resNS, err := BestHand(hole, board, ns)
	if err != nil {
		t.Fatalf("Nova Scotia BestHand: %v", err)
	}
	if resNS.Rank == 1 {
		t.Errorf("Nova Scotia rank=1 (royal): hand-cap violated")
	}

	// Compare to Miami (allows up to 5 from hand): same cards must
	// produce a much stronger result (royal flush, rank 1).
	miami, _ := VariantByKey("miami")
	miamiHole := MustParseCards("As Ks Qs Js Ts") // Miami uses 5-card hand
	resMiami, err := BestHand(miamiHole, board, miami)
	if err != nil {
		t.Fatalf("Miami BestHand: %v", err)
	}
	if resMiami.Rank != 1 {
		t.Errorf("Miami rank=%d want 1 (royal flush from hand)", resMiami.Rank)
	}
	if resMiami.Rank >= resNS.Rank {
		t.Errorf("Miami should beat Nova Scotia: miami=%d ns=%d",
			resMiami.Rank, resNS.Rank)
	}
}

func TestKCKThreeFromHandStraight(t *testing.T) {
	// KCK allows 0-3 from hand. Confirm a 3-from-hand straight resolves.
	kck, _ := VariantByKey("kck")
	hole := MustParseCards("9c 8d 7h")
	board := MustParseCards("6s 5d 2c Kh Qh")

	res, err := BestHand(hole, board, kck)
	if err != nil {
		t.Fatalf("BestHand: %v", err)
	}
	if res.Class != "Straight" {
		t.Errorf("class=%q want Straight (9-8-7-6-5)", res.Class)
	}
}

func TestPortlandRequiresAtLeastOneFromHand(t *testing.T) {
	// Portland forces at least 1 from hand. A board-only royal flush
	// is therefore unreachable; check that the result is weaker than
	// rank 1 even with a royal flush on the board.
	portland, _ := VariantByKey("portland")
	hole := MustParseCards("2c 3d 4h 5s 7h") // garbage
	board := MustParseCards("As Ks Qs Js Ts")

	res, err := BestHand(hole, board, portland)
	if err != nil {
		t.Fatalf("BestHand: %v", err)
	}
	if res.Rank == 1 {
		t.Errorf("Portland reached rank 1 despite forced 1-from-hand")
	}
}

func TestMiamiAllowsBoardOnly(t *testing.T) {
	// Miami allows 0 from hand, so a royal flush on the board plays.
	miami, _ := VariantByKey("miami")
	hole := MustParseCards("2c 3d 4h 5s 7h") // garbage
	board := MustParseCards("As Ks Qs Js Ts")

	res, err := BestHand(hole, board, miami)
	if err != nil {
		t.Fatalf("BestHand: %v", err)
	}
	if res.Rank != 1 {
		t.Errorf("Miami board-only royal: rank=%d want 1", res.Rank)
	}
}

func TestBestHandRejectsWrongHoleSize(t *testing.T) {
	holdem, _ := VariantByKey("holdem")
	board := MustParseCards("As Ks Qs Js Ts")

	if _, err := BestHand(MustParseCards("As"), board, holdem); err == nil {
		t.Error("expected error for too few hole cards")
	}
	if _, err := BestHand(MustParseCards("As Ks Qs"), board, holdem); err == nil {
		t.Error("expected error for too many hole cards")
	}
}

func TestBestHandRejectsWrongBoardSize(t *testing.T) {
	holdem, _ := VariantByKey("holdem")
	hole := MustParseCards("As Ks")
	if _, err := BestHand(hole, MustParseCards("As Ks Qs Js"), holdem); err == nil {
		t.Error("expected error for board < 5 cards")
	}
}
