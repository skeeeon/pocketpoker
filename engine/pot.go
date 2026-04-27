package engine

import (
	"fmt"
	"sort"
)

// goToShowdown finishes the hand: validates side-pot prerequisites,
// evaluates each remaining player's best 5-card hand, distributes the
// pot, and marks the hand complete.
//
// Side pots are intentionally NOT implemented in v1. If at showdown
// any two non-folded seats have committed unequal totals across the
// hand, this function returns an error so the caller can surface a
// clear "side pots not yet supported" message. This catches multi-way
// unequal-stack all-ins; common cases (last-player-standing, equal-stack
// all-ins) settle correctly.
func goToShowdown(s *HandState) error {
	s.Phase = PhaseShowdown
	s.CurrentActorSeat = -1
	s.CurrentBet = 0

	if err := checkNoSidePot(s); err != nil {
		return err
	}

	variant, err := VariantByKey(s.VariantKey)
	if err != nil {
		return err
	}

	type contestant struct {
		seat   int
		result HandResult
	}
	var contestants []contestant
	for i := range s.Players {
		p := &s.Players[i]
		if p.Status == PlayerFolded {
			continue
		}
		res, err := BestHand(p.HoleCards, s.Board, variant)
		if err != nil {
			return fmt.Errorf("showdown seat %d: %w", p.Seat, err)
		}
		contestants = append(contestants, contestant{p.Seat, res})
	}
	if len(contestants) == 0 {
		return fmt.Errorf("showdown: no contestants")
	}

	// Lowest rank wins (1 = royal flush, 7462 = worst high card).
	minRank := contestants[0].result.Rank
	for _, c := range contestants {
		if c.result.Rank < minRank {
			minRank = c.result.Rank
		}
	}
	var winners []contestant
	for _, c := range contestants {
		if c.result.Rank == minRank {
			winners = append(winners, c)
		}
	}

	// Stable seat order so split-pot remainder always goes to the same seat
	// (lowest seat number).
	sort.Slice(winners, func(i, j int) bool {
		return winners[i].seat < winners[j].seat
	})

	share := s.Pot / len(winners)
	remainder := s.Pot - share*len(winners)

	results := make([]SeatResult, len(winners))
	for i, w := range winners {
		amount := share
		if i == 0 {
			amount += remainder
		}
		results[i] = SeatResult{
			Seat:   w.seat,
			Cards:  w.result.Cards,
			Rank:   w.result.Rank,
			Class:  w.result.Class,
			Amount: amount,
		}
		idx, err := s.playerIndex(w.seat)
		if err != nil {
			return err
		}
		s.Players[idx].Stack += amount
	}
	s.Winners = results
	s.Pot = 0
	s.Phase = PhaseComplete
	return nil
}

// checkNoSidePot returns an error if any two non-folded players have
// committed different total amounts across the hand. That case requires
// side-pot accounting which is deferred to v1.1.
func checkNoSidePot(s *HandState) error {
	canonical := -1
	canonicalSeat := -1
	for i := range s.Players {
		p := &s.Players[i]
		if p.Status == PlayerFolded {
			continue
		}
		c := totalCommitted(s, p.Seat)
		if canonical < 0 {
			canonical = c
			canonicalSeat = p.Seat
			continue
		}
		if c != canonical {
			return fmt.Errorf(
				"side pots not yet supported: seat %d committed %d, seat %d committed %d",
				canonicalSeat, canonical, p.Seat, c)
		}
	}
	return nil
}

// totalCommitted returns the total amount `seat` has put into the pot
// across the entire hand (sum of all action amounts for that seat).
func totalCommitted(s *HandState, seat int) int {
	sum := 0
	for _, a := range s.Actions {
		if a.Seat == seat {
			sum += a.Amount
		}
	}
	return sum
}
