package engine

import (
	"fmt"

	cpoker "github.com/chehsunliu/poker"
)

// HandRank is the strength of a 5-card poker hand. Lower is better
// (1 = royal flush, 7462 = worst high card). Mirrors chehsunliu/poker.
type HandRank int32

// HandResult describes a player's best 5-card hand for a given variant.
type HandResult struct {
	Rank   HandRank
	Cards  []Card // the 5 cards that produced Rank
	Class  string // human-readable class, e.g. "Flush"
}

// BestHand returns the best 5-card hand for the given hole + board
// under the variant's combo rules. Returns an error if the hole card
// count does not match the variant or no valid combo can be formed.
func BestHand(hole, board []Card, variant Variant) (HandResult, error) {
	if len(hole) != variant.HandSize {
		return HandResult{}, fmt.Errorf(
			"variant %s expects %d hole cards, got %d",
			variant.Key, variant.HandSize, len(hole))
	}
	if len(board) != BoardSize {
		return HandResult{}, fmt.Errorf(
			"showdown expects %d board cards, got %d",
			BoardSize, len(board))
	}

	combos := variant.ValidCombos()
	if len(combos) == 0 {
		return HandResult{}, fmt.Errorf(
			"variant %s has no valid combos", variant.Key)
	}

	bestRank := HandRank(-1)
	var bestCards []Card

	for _, hb := range combos {
		h, b := hb[0], hb[1]
		holeCombos := combinations(hole, h)
		boardCombos := combinations(board, b)
		for _, hc := range holeCombos {
			for _, bc := range boardCombos {
				five := make([]Card, 0, 5)
				five = append(five, hc...)
				five = append(five, bc...)
				rank := evaluateFive(five)
				if bestRank < 0 || rank < bestRank {
					bestRank = rank
					bestCards = append(bestCards[:0], five...)
				}
			}
		}
	}

	if bestRank < 0 {
		return HandResult{}, fmt.Errorf(
			"no valid 5-card hand for variant %s", variant.Key)
	}

	return HandResult{
		Rank:  bestRank,
		Cards: bestCards,
		Class: cpoker.RankString(int32(bestRank)),
	}, nil
}

// evaluateFive ranks a 5-card hand via the chehsunliu/poker evaluator.
// Lower return value = stronger hand.
func evaluateFive(cards []Card) HandRank {
	cc := make([]cpoker.Card, 5)
	for i, c := range cards {
		cc[i] = cpoker.NewCard(c.String())
	}
	return HandRank(cpoker.Evaluate(cc))
}

// combinations returns all k-element subsets of cards in lexicographic
// (index) order. Allocates a fresh slice per combination so callers can
// retain references. For k=0 returns a single empty slice.
func combinations(cards []Card, k int) [][]Card {
	n := len(cards)
	if k < 0 || k > n {
		return nil
	}
	if k == 0 {
		return [][]Card{{}}
	}
	if k == n {
		out := make([]Card, n)
		copy(out, cards)
		return [][]Card{out}
	}

	var result [][]Card
	indices := make([]int, k)
	for i := range indices {
		indices[i] = i
	}
	for {
		combo := make([]Card, k)
		for i, idx := range indices {
			combo[i] = cards[idx]
		}
		result = append(result, combo)

		// Advance the indices: rightmost that can move forward.
		i := k - 1
		for i >= 0 && indices[i] == n-k+i {
			i--
		}
		if i < 0 {
			break
		}
		indices[i]++
		for j := i + 1; j < k; j++ {
			indices[j] = indices[j-1] + 1
		}
	}
	return result
}
