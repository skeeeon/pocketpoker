package engine

import "testing"

func TestVariantsHaveUniqueKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range Variants {
		if seen[v.Key] {
			t.Errorf("duplicate variant key %q", v.Key)
		}
		seen[v.Key] = true
	}
}

func TestVariantByKey(t *testing.T) {
	v, err := VariantByKey("holdem")
	if err != nil {
		t.Fatalf("VariantByKey holdem: %v", err)
	}
	if v.HandSize != 2 {
		t.Errorf("holdem HandSize=%d want 2", v.HandSize)
	}
	if _, err := VariantByKey("nope"); err == nil {
		t.Error("VariantByKey on unknown key should error")
	}
}

func TestVariantMaxSeatsMatchesDeckSize(t *testing.T) {
	// MaxSeats in the config table must not exceed what a 52-card deck
	// can support: (52 - 5 community) / hand_size.
	for _, v := range Variants {
		ceiling := v.computedMaxSeats()
		if v.MaxSeats > ceiling {
			t.Errorf("variant %s: MaxSeats=%d exceeds deck ceiling %d",
				v.Key, v.MaxSeats, ceiling)
		}
		// Sanity: 7-card variants must cap at 6.
		if v.HandSize == 7 && v.MaxSeats != 6 {
			t.Errorf("variant %s: 7-card hand must cap at 6 seats, got %d",
				v.Key, v.MaxSeats)
		}
	}
}

func TestCheckSeatFits(t *testing.T) {
	holdem, _ := VariantByKey("holdem")
	if err := holdem.CheckSeatFits(8); err != nil {
		t.Errorf("holdem 8 seats should fit: %v", err)
	}
	if err := holdem.CheckSeatFits(9); err == nil {
		t.Error("holdem 9 seats should not fit (max_seats=8)")
	}
	if err := holdem.CheckSeatFits(0); err == nil {
		t.Error("0 seats should error")
	}

	dubai, _ := VariantByKey("dubai")
	if err := dubai.CheckSeatFits(6); err != nil {
		t.Errorf("dubai 6 seats should fit: %v", err)
	}
	if err := dubai.CheckSeatFits(7); err == nil {
		t.Error("dubai 7 seats should not fit (max_seats=6)")
	}
}

func TestValidCombosByVariant(t *testing.T) {
	cases := []struct {
		key  string
		want [][2]int
	}{
		// Hold'em: 0-2 from hand, 3-5 from board, summing to 5.
		// (0,5), (1,4), (2,3).
		{"holdem", [][2]int{{0, 5}, {1, 4}, {2, 3}}},
		// Omaha: exactly 2 from hand, 3 from board.
		{"omaha", [][2]int{{2, 3}}},
		// KCK: 0-3 from hand, 2-5 from board. (0,5), (1,4), (2,3), (3,2).
		{"kck", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}}},
		// Kansas City: 0-4 from hand, 1-5 from board. (0,5)-(4,1).
		{"kansas_city", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}, {4, 1}}},
		// Portland: 0-4 from hand, 1-5 from board. Like Miami but caps
		// hand usage at 4 (cannot play all 5 hole cards).
		{"portland", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}, {4, 1}}},
		// Miami: 0-5 from hand, 0-5 from board. All sums-to-5 pairs.
		{"miami", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}, {4, 1}, {5, 0}}},
		// Three Fifths: 5-card hand, 0-3 from hand.
		{"three_fifths", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}}},
		// St Louis: 6-card hand, 0-3 from hand.
		{"st_louis", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}}},
		// Dubai: hand size 7 but at most 5 used; same as Miami in (h,b) space.
		{"dubai", [][2]int{{0, 5}, {1, 4}, {2, 3}, {3, 2}, {4, 1}, {5, 0}}},
		// Nova Scotia: 0-2 from hand, 3-5 from board. Same shape as Hold'em.
		{"nova_scotia", [][2]int{{0, 5}, {1, 4}, {2, 3}}},
	}
	for _, tc := range cases {
		v, err := VariantByKey(tc.key)
		if err != nil {
			t.Fatalf("%s: %v", tc.key, err)
		}
		got := v.ValidCombos()
		if !sameCombos(got, tc.want) {
			t.Errorf("%s ValidCombos=%v want %v", tc.key, got, tc.want)
		}
	}
}

func sameCombos(a, b [][2]int) bool {
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
