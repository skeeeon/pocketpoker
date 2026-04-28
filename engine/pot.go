package engine

import (
	"fmt"
	"sort"
)

// goToShowdown finishes the hand: builds the main pot plus any side
// pots from the action log, evaluates each non-folded player's best
// 5-card hand, distributes each pot to the best hand among its
// eligible seats, and marks the hand complete.
//
// Side-pot layering: when seats committed unequal totals (typically
// because someone went all-in for less than the full bet), the chips
// split into layers. Each layer is contested only by seats whose
// total commitment reached that layer; chips from a folded or
// short-stacked seat below the layer still seed the pot but their
// owner cannot win it.
func goToShowdown(s *HandState) error {
	s.Phase = PhaseShowdown
	s.CurrentActorSeat = -1
	s.CurrentBet = 0

	variant, err := VariantByKey(s.VariantKey)
	if err != nil {
		return err
	}

	// Evaluate each non-folded seat once and reuse across all pots.
	results := make(map[int]HandResult)
	for i := range s.Players {
		p := &s.Players[i]
		if p.Status == PlayerFolded {
			continue
		}
		res, err := BestHand(p.HoleCards, s.Board, variant)
		if err != nil {
			return fmt.Errorf("showdown seat %d: %w", p.Seat, err)
		}
		results[p.Seat] = res
	}
	if len(results) == 0 {
		return fmt.Errorf("showdown: no contestants")
	}

	pots := buildPots(s)

	// Per-seat aggregated payout for the legacy Winners field. Keyed
	// by seat so a winner who collects from multiple pots gets one
	// row whose Amount is the sum.
	totalBySeat := map[int]int{}

	for pi := range pots {
		pot := &pots[pi]
		// Find best rank among eligible seats. Eligible is constructed
		// from non-folded seats, so every entry has a HandResult.
		minRank := HandRank(0)
		first := true
		for _, seat := range pot.Eligible {
			r := results[seat].Rank
			if first || r < minRank {
				minRank = r
				first = false
			}
		}
		var winners []int
		for _, seat := range pot.Eligible {
			if results[seat].Rank == minRank {
				winners = append(winners, seat)
			}
		}
		// Stable seat order so split-pot remainder always goes to the
		// same seat (lowest seat number).
		sort.Ints(winners)

		share := pot.Amount / len(winners)
		remainder := pot.Amount - share*len(winners)
		potResults := make([]SeatResult, len(winners))
		for i, w := range winners {
			amount := share
			if i == 0 {
				amount += remainder
			}
			r := results[w]
			potResults[i] = SeatResult{
				Seat:   w,
				Cards:  r.Cards,
				Rank:   r.Rank,
				Class:  r.Class,
				Amount: amount,
			}
			idx, err := s.playerIndex(w)
			if err != nil {
				return err
			}
			s.Players[idx].Stack += amount
			totalBySeat[w] += amount
		}
		pot.Winners = potResults
	}

	// Aggregate per-seat winnings across pots into the legacy Winners
	// list. Each seat appears at most once; Cards/Rank/Class come from
	// that seat's evaluated hand.
	winningSeats := make([]int, 0, len(totalBySeat))
	for seat := range totalBySeat {
		winningSeats = append(winningSeats, seat)
	}
	sort.Ints(winningSeats)
	aggregated := make([]SeatResult, 0, len(winningSeats))
	for _, seat := range winningSeats {
		r := results[seat]
		aggregated = append(aggregated, SeatResult{
			Seat:   seat,
			Cards:  r.Cards,
			Rank:   r.Rank,
			Class:  r.Class,
			Amount: totalBySeat[seat],
		})
	}

	s.Pots = pots
	s.Winners = aggregated
	s.Pot = 0
	s.Phase = PhaseComplete
	return nil
}

// buildPots constructs the main pot and any side pots from the action
// log + folded status. It sweeps the distinct total-commitment levels
// of non-folded seats in ascending order; each level produces one pot
// whose chips come from min(c_j, level) - min(c_j, prev) summed over
// every seat (folded contributors included), and whose eligible
// winners are the non-folded seats with c_j >= level.
//
// In the common case of equal commitments this produces a single pot
// equal to s.Pot. With unequal all-ins it produces the standard
// main + side layering. Uncalled bets fall out naturally: the top
// layer has only one eligible seat, who trivially wins it back.
func buildPots(s *HandState) []Pot {
	commit := make(map[int]int, len(s.Players))
	for _, a := range s.Actions {
		commit[a.Seat] += a.Amount
	}

	levelSet := map[int]struct{}{}
	for i := range s.Players {
		p := &s.Players[i]
		if p.Status == PlayerFolded {
			continue
		}
		if c := commit[p.Seat]; c > 0 {
			levelSet[c] = struct{}{}
		}
	}
	levels := make([]int, 0, len(levelSet))
	for l := range levelSet {
		levels = append(levels, l)
	}
	sort.Ints(levels)

	var pots []Pot
	prev := 0
	for _, level := range levels {
		amount := 0
		for _, c := range commit {
			contribution := min(c, level) - min(c, prev)
			if contribution > 0 {
				amount += contribution
			}
		}
		var eligible []int
		for i := range s.Players {
			p := &s.Players[i]
			if p.Status == PlayerFolded {
				continue
			}
			if commit[p.Seat] >= level {
				eligible = append(eligible, p.Seat)
			}
		}
		sort.Ints(eligible)
		if amount > 0 && len(eligible) > 0 {
			pots = append(pots, Pot{Amount: amount, Eligible: eligible})
		}
		prev = level
	}
	return pots
}

