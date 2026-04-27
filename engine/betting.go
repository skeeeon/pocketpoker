package engine

import "fmt"

// ApplyAction applies a single action to the hand and returns the new
// state. Pure: the input state is unmodified on either success or error.
//
// The caller passes (seat, type, amount):
//   - seat must equal state.CurrentActorSeat.
//   - amount semantics depend on type:
//       fold/check/all_in:  amount=0 (or for all_in, optionally the full stack)
//       call:               amount is ignored; the engine moves exactly the
//                           outstanding-to-call (or stack, whichever is less)
//       bet/raise:          amount is the chips added on top of what `seat`
//                           has already committed this round
func ApplyAction(state HandState, seat int, actionType ActionType, amount int) (HandState, error) {
	s := state // value copy; we mutate this and return on success

	if s.Phase >= PhaseShowdown {
		return state, fmt.Errorf("apply: hand already at phase %s", s.Phase)
	}
	if seat != s.CurrentActorSeat {
		return state, fmt.Errorf(
			"apply: not seat %d's turn (current=%d)", seat, s.CurrentActorSeat)
	}

	idx, err := s.playerIndex(seat)
	if err != nil {
		return state, err
	}
	p := &s.Players[idx]
	if p.Status != PlayerActive {
		return state, fmt.Errorf(
			"apply: seat %d is %s, cannot act", seat, p.Status)
	}

	committed := s.roundCommit(seat)
	toCall := s.CurrentBet - committed
	if toCall < 0 {
		toCall = 0
	}

	var actualAmount int
	var newStatus PlayerStatus
	var recordedType = actionType

	switch actionType {
	case ActionFold:
		if amount != 0 {
			return state, fmt.Errorf("apply: fold must have amount=0, got %d", amount)
		}
		actualAmount = 0
		newStatus = PlayerFolded

	case ActionCheck:
		if amount != 0 {
			return state, fmt.Errorf("apply: check must have amount=0, got %d", amount)
		}
		if toCall > 0 {
			return state, fmt.Errorf(
				"apply: cannot check with %d to call", toCall)
		}
		actualAmount = 0
		newStatus = PlayerActive

	case ActionCall:
		if toCall <= 0 {
			return state, fmt.Errorf("apply: nothing to call (current_bet=%d, committed=%d)",
				s.CurrentBet, committed)
		}
		actualAmount = toCall
		if actualAmount > p.Stack {
			actualAmount = p.Stack
		}
		if actualAmount == p.Stack {
			newStatus = PlayerAllIn
			recordedType = ActionAllIn
		} else {
			newStatus = PlayerActive
		}

	case ActionBet:
		if s.CurrentBet > 0 {
			return state, fmt.Errorf(
				"apply: cannot bet when current_bet=%d, must raise", s.CurrentBet)
		}
		if amount <= 0 {
			return state, fmt.Errorf("apply: bet amount must be positive, got %d", amount)
		}
		if amount > p.Stack {
			return state, fmt.Errorf("apply: bet %d exceeds stack %d", amount, p.Stack)
		}
		if amount < s.BigBlind && amount != p.Stack {
			return state, fmt.Errorf(
				"apply: bet %d below big blind %d (or go all-in)", amount, s.BigBlind)
		}
		actualAmount = amount
		if actualAmount == p.Stack {
			newStatus = PlayerAllIn
			recordedType = ActionAllIn
		} else {
			newStatus = PlayerActive
		}

	case ActionRaise:
		if s.CurrentBet == 0 {
			return state, fmt.Errorf("apply: cannot raise when no bet, use bet")
		}
		if amount <= 0 {
			return state, fmt.Errorf("apply: raise amount must be positive, got %d", amount)
		}
		if amount > p.Stack {
			return state, fmt.Errorf("apply: raise %d exceeds stack %d", amount, p.Stack)
		}
		newTotal := committed + amount
		if newTotal <= s.CurrentBet {
			return state, fmt.Errorf(
				"apply: raise total %d does not exceed current bet %d",
				newTotal, s.CurrentBet)
		}
		minRaiseTotal := s.CurrentBet + s.BigBlind
		isAllIn := amount == p.Stack
		if newTotal < minRaiseTotal && !isAllIn {
			return state, fmt.Errorf(
				"apply: raise total %d below minimum %d (or go all-in)",
				newTotal, minRaiseTotal)
		}
		actualAmount = amount
		if isAllIn {
			newStatus = PlayerAllIn
			recordedType = ActionAllIn
		} else {
			newStatus = PlayerActive
		}

	case ActionAllIn:
		if p.Stack <= 0 {
			return state, fmt.Errorf("apply: stack is 0, cannot all-in")
		}
		if amount != 0 && amount != p.Stack {
			return state, fmt.Errorf(
				"apply: all-in amount must be 0 or full stack %d, got %d",
				p.Stack, amount)
		}
		actualAmount = p.Stack
		newStatus = PlayerAllIn

	case ActionPostBlind:
		return state, fmt.Errorf("apply: post_blind is internal-only")

	default:
		return state, fmt.Errorf("apply: unknown action type %q", actionType)
	}

	// Mutate.
	p.Stack -= actualAmount
	p.Status = newStatus
	s.Pot += actualAmount

	s.Actions = append(s.Actions, Action{
		Sequence: len(s.Actions) + 1,
		Seat:     seat,
		Phase:    s.Phase,
		Type:     recordedType,
		Amount:   actualAmount,
	})

	newTotal := committed + actualAmount
	if newTotal > s.CurrentBet {
		s.CurrentBet = newTotal
	}

	if err := advanceTurn(&s); err != nil {
		return state, err
	}

	return s, nil
}

// advanceTurn updates state.CurrentActorSeat to the next active seat,
// or advances the phase when the round is complete.
func advanceTurn(s *HandState) error {
	if s.countLive() <= 1 {
		return finalizeFoldedOut(s)
	}

	if isRoundComplete(s) {
		return advancePhase(s)
	}

	next := s.nextActiveSeatAfter(s.CurrentActorSeat)
	if next < 0 {
		return advancePhase(s)
	}
	s.CurrentActorSeat = next
	return nil
}

// isRoundComplete returns true when every active (not-folded, not-all-in)
// player has voluntarily acted at least once during the current phase
// AND committed at least the current bet for the round.
func isRoundComplete(s *HandState) bool {
	if s.countActive() == 0 {
		return true
	}
	for i := range s.Players {
		p := &s.Players[i]
		if p.Status != PlayerActive {
			continue
		}
		if !s.hasActedVoluntarily(p.Seat) {
			return false
		}
		if s.roundCommit(p.Seat) < s.CurrentBet {
			return false
		}
	}
	return true
}

// finalizeFoldedOut awards the pot to the lone live player when all
// others have folded, and marks the hand complete.
func finalizeFoldedOut(s *HandState) error {
	winnerSeat := -1
	for i := range s.Players {
		p := &s.Players[i]
		if p.Status == PlayerActive || p.Status == PlayerAllIn {
			winnerSeat = p.Seat
			break
		}
	}
	if winnerSeat < 0 {
		return fmt.Errorf("finalize: no live players")
	}

	idx, err := s.playerIndex(winnerSeat)
	if err != nil {
		return err
	}
	pot := s.Pot
	s.Players[idx].Stack += pot
	s.Winners = []SeatResult{{
		Seat:   winnerSeat,
		Cards:  nil,
		Rank:   0,
		Class:  "won uncontested",
		Amount: pot,
	}}
	s.Pot = 0
	s.Phase = PhaseComplete
	s.CurrentActorSeat = -1
	return nil
}

// advancePhase deals the next street(s) and either reopens betting
// or runs out the board straight to showdown if no one can still act.
func advancePhase(s *HandState) error {
	for {
		switch s.Phase {
		case PhasePreflop:
			cards, err := dealFromState(s, 3)
			if err != nil {
				return fmt.Errorf("deal flop: %w", err)
			}
			s.Board = append(s.Board, cards...)
			s.Phase = PhaseFlop

		case PhaseFlop:
			cards, err := dealFromState(s, 1)
			if err != nil {
				return fmt.Errorf("deal turn: %w", err)
			}
			s.Board = append(s.Board, cards...)
			s.Phase = PhaseTurn

		case PhaseTurn:
			cards, err := dealFromState(s, 1)
			if err != nil {
				return fmt.Errorf("deal river: %w", err)
			}
			s.Board = append(s.Board, cards...)
			s.Phase = PhaseRiver

		case PhaseRiver:
			return goToShowdown(s)

		default:
			return fmt.Errorf("advancePhase: unexpected phase %s", s.Phase)
		}

		s.CurrentBet = 0

		// If 2+ active players remain, reopen betting on the new street.
		if s.countActive() >= 2 {
			next := s.firstActiveSeatClockwise(s.SmallBlindSeat)
			if next < 0 {
				return fmt.Errorf("advancePhase: no active seat to act on %s", s.Phase)
			}
			s.CurrentActorSeat = next
			return nil
		}
		// Otherwise, no betting possible — keep dealing toward showdown.
	}
}

// dealFromState pulls n cards from the persisted deck snapshot,
// advancing DeckPos in place.
func dealFromState(s *HandState, n int) ([]Card, error) {
	if n < 0 {
		return nil, fmt.Errorf("dealFromState: negative count %d", n)
	}
	if s.DeckPos+n > len(s.Deck) {
		return nil, ErrDeckExhausted
	}
	out := make([]Card, n)
	copy(out, s.Deck[s.DeckPos:s.DeckPos+n])
	s.DeckPos += n
	return out, nil
}
