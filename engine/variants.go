package engine

import "fmt"

// BoardSize is the number of community cards dealt across all phases.
// All supported variants use a 5-card board.
const BoardSize = 5

// Variant describes how a player's best 5-card hand is constructed
// from their hole cards and the community board. The constraint is:
//
//	hand_used + board_used = 5
//	MinFromHand <= hand_used <= MaxFromHand
//	MinFromBoard <= board_used <= MaxFromBoard
//
// MaxSeats is the largest table size that fits in a 52-card deck for
// this variant: floor((52 - BoardSize) / HandSize).
type Variant struct {
	Key           string
	Name          string
	HandSize      int
	MinFromHand   int
	MaxFromHand   int
	MinFromBoard  int
	MaxFromBoard  int
	MaxSeats      int
}

// Variants is the canonical config table. Order is the order shown to
// users in the variant picker.
var Variants = []Variant{
	{"holdem", "Hold'em", 2, 0, 2, 3, 5, 8},
	{"omaha", "Omaha", 4, 2, 2, 3, 3, 8},
	{"kck", "KCK", 3, 0, 3, 2, 5, 8},
	{"kansas_city", "Kansas City", 4, 0, 4, 1, 5, 8},
	{"portland", "Portland", 5, 0, 4, 1, 5, 8},
	{"miami", "Miami", 5, 0, 5, 0, 5, 8},
	{"three_fifths", "Three Fifths", 5, 0, 3, 2, 5, 8},
	{"st_louis", "St Louis", 6, 0, 3, 2, 5, 7},
	{"dubai", "Dubai", 7, 0, 5, 0, 5, 6},
	{"nova_scotia", "Nova Scotia", 7, 0, 2, 3, 5, 6},
}

// VariantByKey returns the variant config for the given key, or an
// error if no variant matches.
func VariantByKey(key string) (Variant, error) {
	for _, v := range Variants {
		if v.Key == key {
			return v, nil
		}
	}
	return Variant{}, fmt.Errorf("unknown variant %q", key)
}

// ValidCombos enumerates all (handUsed, boardUsed) pairs allowed by the
// variant's constraints. Each pair satisfies handUsed + boardUsed = 5.
func (v Variant) ValidCombos() [][2]int {
	var out [][2]int
	for h := v.MinFromHand; h <= v.MaxFromHand; h++ {
		b := 5 - h
		if b < v.MinFromBoard || b > v.MaxFromBoard {
			continue
		}
		if h > v.HandSize {
			continue
		}
		if b > BoardSize {
			continue
		}
		out = append(out, [2]int{h, b})
	}
	return out
}

// CheckSeatFits reports whether the given seat count fits in a 52-card
// deck for this variant.
func (v Variant) CheckSeatFits(seats int) error {
	if seats <= 0 {
		return fmt.Errorf("seats must be positive, got %d", seats)
	}
	if seats > v.MaxSeats {
		return fmt.Errorf("variant %s supports at most %d seats, got %d",
			v.Key, v.MaxSeats, seats)
	}
	return nil
}

// computedMaxSeats is the deck-size ceiling implied by HandSize. Used
// internally to verify the static config is consistent.
func (v Variant) computedMaxSeats() int {
	if v.HandSize <= 0 {
		return 0
	}
	return (DeckSize - BoardSize) / v.HandSize
}
