package engine

import (
	"math/rand"
)

// BotPersonality is the knobs that distinguish one bot's style from
// another. All values live in [0,1] (BluffFreq is small in practice).
//   - Aggression: probability of raising vs. just calling when ahead.
//   - Looseness:  shrinks both the pot-odds threshold and the absolute
//     equity floor, so looser bots fold less.
//   - BluffFreq:  probability of betting/raising into a checked-down or
//     mildly-strong hand. Caps at 0.2 in practice.
type BotPersonality struct {
	Name       string
	Aggression float64
	Looseness  float64
	BluffFreq  float64
}

// Personalities is the canonical archetype set. Keys match the values
// of the seats.bot_personality select field.
var Personalities = map[string]BotPersonality{
	"tight":   {Name: "Tight Tina", Aggression: 0.5, Looseness: 0.30, BluffFreq: 0.05},
	"loose":   {Name: "Loose Larry", Aggression: 0.4, Looseness: 0.70, BluffFreq: 0.10},
	"maniac":  {Name: "Maniac Mike", Aggression: 0.9, Looseness: 0.60, BluffFreq: 0.20},
	"station": {Name: "Calling Station Carl", Aggression: 0.1, Looseness: 0.80, BluffFreq: 0.00},
}

// equitySamples is the Monte Carlo trial count used by Decide. Higher
// is more accurate, lower is faster. 200 strikes a good balance: <50ms
// even for the 7-card variants and resolution well below the 0.05
// difference our personality thresholds care about.
const equitySamples = 200

// Equity estimates the probability that `seat` will win the hand at
// showdown given the currently visible state, sampling `samples` random
// fillings of opponent hole cards and remaining board cards. Result is
// in [0,1] where 0.5 includes a half-credit per chopped pot. The bot
// does NOT peek at opponents' real hole cards or unseen deck cards.
func Equity(state HandState, seat int, samples int, rng *rand.Rand) float64 {
	variant, err := VariantByKey(state.VariantKey)
	if err != nil {
		return 0.5
	}
	myIdx, err := state.playerIndex(seat)
	if err != nil {
		return 0.5
	}
	myHole := state.Players[myIdx].HoleCards
	if len(myHole) != variant.HandSize {
		return 0.5
	}

	// Active live opponents = anyone not us and not folded. All-ins still
	// reach showdown so they count.
	oppIdx := make([]int, 0, len(state.Players))
	for i, p := range state.Players {
		if i == myIdx {
			continue
		}
		if p.Status == PlayerFolded {
			continue
		}
		oppIdx = append(oppIdx, i)
	}
	if len(oppIdx) == 0 {
		return 1.0
	}

	// Build the pool of cards we don't know about: the full 52 minus our
	// hole + the visible board.
	seen := make(map[Card]bool, len(myHole)+len(state.Board))
	for _, c := range myHole {
		seen[c] = true
	}
	for _, c := range state.Board {
		seen[c] = true
	}
	unseen := make([]Card, 0, DeckSize)
	for s := Clubs; s <= Spades; s++ {
		for r := Two; r <= Ace; r++ {
			c := Card{Rank: r, Suit: s}
			if !seen[c] {
				unseen = append(unseen, c)
			}
		}
	}

	needBoard := BoardSize - len(state.Board)
	needOpp := variant.HandSize * len(oppIdx)
	if needBoard+needOpp > len(unseen) {
		return 0.5
	}

	wins := 0.0
	board := make([]Card, BoardSize)
	copy(board, state.Board)

	for s := 0; s < samples; s++ {
		// Partial Fisher-Yates: only shuffle as much as we need.
		for i := 0; i < needBoard+needOpp; i++ {
			j := i + rng.Intn(len(unseen)-i)
			unseen[i], unseen[j] = unseen[j], unseen[i]
		}

		// Fill in the missing board.
		copy(board[len(state.Board):], unseen[:needBoard])

		me, err := BestHand(myHole, board, variant)
		if err != nil {
			continue
		}

		bestOpp := HandRank(-1)
		ties := 0
		cursor := needBoard
		for range oppIdx {
			oppHole := unseen[cursor : cursor+variant.HandSize]
			cursor += variant.HandSize
			opp, err := BestHand(oppHole, board, variant)
			if err != nil {
				continue
			}
			if bestOpp < 0 || opp.Rank < bestOpp {
				bestOpp = opp.Rank
				ties = 0
			} else if opp.Rank == bestOpp {
				ties++
			}
		}

		switch {
		case me.Rank < bestOpp:
			wins += 1.0
		case me.Rank == bestOpp:
			// Split among me + everyone tied with bestOpp.
			wins += 1.0 / float64(2+ties)
		}
	}
	return wins / float64(samples)
}

// Decide returns the bot's chosen action for the current state. The
// caller must guarantee `seat == state.CurrentActorSeat` and that the
// hand is in a betting phase. The returned (type, amount) pair is
// always legal under engine.ApplyAction; the engine may recode a raise
// equal to the full stack as ActionAllIn, which is fine.
func Decide(state HandState, seat int, p BotPersonality, rng *rand.Rand) (ActionType, int) {
	idx, err := state.playerIndex(seat)
	if err != nil {
		return ActionFold, 0
	}
	me := state.Players[idx]
	if me.Status != PlayerActive {
		return ActionFold, 0
	}

	committed := state.roundCommit(seat)
	toCall := state.CurrentBet - committed
	if toCall < 0 {
		toCall = 0
	}

	equity := Equity(state, seat, equitySamples, rng)

	potOdds := 0.0
	if toCall > 0 {
		potOdds = float64(toCall) / float64(state.Pot+toCall)
	}
	// Threshold combines pot odds (must be paying off in expectation) and
	// an absolute equity floor (don't call off chips on a coin flip just
	// because the price is right). Looseness shrinks both knobs.
	//
	// Fair-share equity vs N random opponents averages ~1/(N+1), so the
	// floor must scale with the live field — a 0.5-anchored floor turns
	// every bot into a nit at full ring (e.g. 6-handed Dubai, where fair
	// share is ~14% and even Loose Larry's old 0.29 floor was unreachable).
	liveOpps := 0
	for i, pl := range state.Players {
		if i == idx || pl.Status == PlayerFolded {
			continue
		}
		liveOpps++
	}
	fairShare := 1.0 / float64(liveOpps+1)
	floorMult := 1.3 - p.Looseness
	if floorMult < 0.5 {
		floorMult = 0.5
	}
	threshold := potOdds * (1 - p.Looseness)
	if floor := fairShare * floorMult; floor > threshold {
		threshold = floor
	}

	// Equity tiers for action selection, scaled by fair share so the
	// "I'm comfortably ahead" calls still trigger multi-way. Bluff is
	// less aggressively scaled (with a 0.20 absolute floor) so full-ring
	// bots don't fire bluffs at every sub-fair-share holding. All three
	// reduce to the original heads-up constants when liveOpps == 1.
	valueBetTier := 1.1 * fairShare
	raiseTier := 1.2 * fairShare
	bluffTier := 0.7 * fairShare
	if bluffTier < 0.20 {
		bluffTier = 0.20
	}

	hasRaised := false
	for _, a := range state.Actions {
		if a.Phase == state.Phase && a.Seat == seat &&
			(a.Type == ActionRaise || a.Type == ActionBet) {
			hasRaised = true
			break
		}
	}

	// Free option: no chips owed. Check unless we want to value-bet or bluff.
	if toCall == 0 {
		if !hasRaised {
			valueBet := equity > valueBetTier && rng.Float64() < p.Aggression
			bluff := equity > bluffTier && rng.Float64() < p.BluffFreq
			if valueBet || bluff {
				return sizedBetOrRaise(state, seat, p, equity)
			}
		}
		return ActionCheck, 0
	}

	// Facing a bet. Fold below threshold, with a small bluff-raise chance
	// for personalities that have BluffFreq > 0.
	if equity < threshold {
		if !hasRaised && equity > 0.2 && rng.Float64() < p.BluffFreq {
			return sizedBetOrRaise(state, seat, p, equity)
		}
		return ActionFold, 0
	}

	// Strong enough to continue. Raise occasionally, otherwise call.
	if !hasRaised && equity > raiseTier && rng.Float64() < p.Aggression {
		return sizedBetOrRaise(state, seat, p, equity)
	}
	return ActionCall, 0
}

// sizedBetOrRaise produces a pot-fraction-sized bet or raise. Falls
// through to call/check when the bot can't legally raise (e.g., size
// would be smaller than min-raise and we're not jamming).
func sizedBetOrRaise(state HandState, seat int, p BotPersonality, equity float64) (ActionType, int) {
	idx, err := state.playerIndex(seat)
	if err != nil {
		return ActionFold, 0
	}
	me := state.Players[idx]
	committed := state.roundCommit(seat)

	// Pot-fraction sizing: half-pot baseline, plus aggression*equity for
	// the strong-and-aggro case, capped at pot.
	frac := 0.5 + 0.5*p.Aggression*equity
	if frac > 1.0 {
		frac = 1.0
	}

	if state.CurrentBet == 0 {
		size := int(float64(state.Pot) * frac)
		if size < state.BigBlind {
			size = state.BigBlind
		}
		if size > me.Stack {
			size = me.Stack
		}
		if size < state.BigBlind && size != me.Stack {
			// Stack is below BB; only legal move is shove. ApplyAction
			// will recode the full-stack bet as AllIn.
			return ActionBet, me.Stack
		}
		return ActionBet, size
	}

	// Raise. Engine wants `amount` = chips on top of what we've already
	// committed; newTotal = committed + amount must be >= currentBet + BB.
	raiseTo := state.CurrentBet + int(float64(state.Pot+state.CurrentBet)*frac)
	minRaiseTotal := state.CurrentBet + state.BigBlind
	if raiseTo < minRaiseTotal {
		raiseTo = minRaiseTotal
	}
	amount := raiseTo - committed
	if amount > me.Stack {
		amount = me.Stack
	}
	if amount <= state.CurrentBet-committed {
		// Can't legally raise (would be under min-raise and not all-in).
		// Fall back to a plain call.
		return ActionCall, 0
	}
	return ActionRaise, amount
}
