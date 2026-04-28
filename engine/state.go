package engine

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Phase enumerates the lifecycle of a hand.
type Phase int

const (
	PhasePreflop Phase = iota
	PhaseFlop
	PhaseTurn
	PhaseRiver
	PhaseShowdown
	PhaseComplete
)

var phaseStrings = [...]string{"preflop", "flop", "turn", "river", "showdown", "complete"}

func (p Phase) String() string {
	if int(p) < 0 || int(p) >= len(phaseStrings) {
		return "unknown"
	}
	return phaseStrings[p]
}

// MarshalJSON serialises Phase as its lowercase label so the wire
// representation matches the SelectField on hands.phase. Without this,
// embedded actions[].phase ships as an int and frontend filters that
// compare against the string label silently never match.
func (p Phase) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// UnmarshalJSON accepts either the string label (preferred) or the
// legacy integer encoding so older persisted rows still load.
func (p *Phase) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		for i, label := range phaseStrings {
			if label == s {
				*p = Phase(i)
				return nil
			}
		}
		return fmt.Errorf("unknown phase %q", s)
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("phase: expected string or int, got %s", string(data))
	}
	if n < 0 || n >= len(phaseStrings) {
		return fmt.Errorf("phase int out of range: %d", n)
	}
	*p = Phase(n)
	return nil
}

// ActionType is the kind of action a player takes during a hand.
type ActionType string

const (
	ActionPostBlind ActionType = "post_blind"
	ActionFold      ActionType = "fold"
	ActionCheck     ActionType = "check"
	ActionCall      ActionType = "call"
	ActionBet       ActionType = "bet"
	ActionRaise     ActionType = "raise"
	ActionAllIn     ActionType = "all_in"
)

// Action is a single recorded move in a hand. Append-only.
type Action struct {
	Sequence int        `json:"sequence"`
	Seat     int        `json:"seat"`
	Phase    Phase      `json:"phase"`
	Type     ActionType `json:"type"`
	Amount   int        `json:"amount"`
}

// PlayerStatus represents whether a player is still in the hand.
type PlayerStatus string

const (
	PlayerActive PlayerStatus = "active"
	PlayerFolded PlayerStatus = "folded"
	PlayerAllIn  PlayerStatus = "all_in"
)

// PlayerState is a single seat's view of an in-progress hand.
type PlayerState struct {
	Seat      int          `json:"seat"`
	HoleCards []Card       `json:"hole_cards"`
	Stack     int          `json:"stack"`
	Status    PlayerStatus `json:"status"`
}

// SeatResult records a single seat's showdown outcome.
type SeatResult struct {
	Seat   int      `json:"seat"`
	Cards  []Card   `json:"cards"`
	Rank   HandRank `json:"rank"`
	Class  string   `json:"class"`
	Amount int      `json:"amount"`
}

// Pot is a single chip layer at showdown. With unequal all-ins, the
// total wager splits into a main pot plus one or more side pots; each
// is contested only by seats whose total commitment reached that layer.
type Pot struct {
	Amount   int          `json:"amount"`
	Eligible []int        `json:"eligible"`
	Winners  []SeatResult `json:"winners,omitempty"`
}

// HandState captures everything about an in-progress hand. It is the
// engine's single source of truth and the wire shape persisted to
// PocketBase. Pure transition functions accept and return HandState.
type HandState struct {
	VariantKey       string        `json:"variant_key"`
	Deck             []Card        `json:"deck_state"`
	DeckPos          int           `json:"deck_pos"`
	Board            []Card        `json:"board"`
	Players          []PlayerState `json:"players"`
	Pot              int           `json:"pot"`
	Phase            Phase         `json:"phase"`
	DealerSeat       int           `json:"dealer_seat"`
	SmallBlindSeat   int           `json:"small_blind_seat"`
	BigBlindSeat     int           `json:"big_blind_seat"`
	CurrentActorSeat int           `json:"current_actor_seat"`
	CurrentBet       int           `json:"current_bet"`
	SmallBlind       int           `json:"small_blind"`
	BigBlind         int           `json:"big_blind"`
	Actions          []Action      `json:"actions"`
	Winners          []SeatResult  `json:"winners,omitempty"`
	Pots             []Pot         `json:"pots,omitempty"`
}

// SeatedPlayer is the input to Deal: a seat with its starting stack.
type SeatedPlayer struct {
	Seat  int
	Stack int
}

// MinPlayers is the minimum number of players a hand requires. v1
// does not implement heads-up special ordering.
const MinPlayers = 3

// Deal sets up a new hand: validates inputs, posts blinds, deals hole
// cards, and sets the first actor. The deck must already be shuffled.
// dealerSeat must be the seat number of one of the players.
func Deal(
	variant Variant,
	players []SeatedPlayer,
	deck *Deck,
	smallBlind, bigBlind int,
	dealerSeat int,
) (HandState, error) {
	if deck == nil {
		return HandState{}, fmt.Errorf("deal: nil deck")
	}
	if len(players) < MinPlayers {
		return HandState{}, fmt.Errorf("deal: need at least %d players, got %d",
			MinPlayers, len(players))
	}
	if err := variant.CheckSeatFits(len(players)); err != nil {
		return HandState{}, fmt.Errorf("deal: %w", err)
	}
	if smallBlind <= 0 || bigBlind <= 0 || smallBlind >= bigBlind {
		return HandState{}, fmt.Errorf(
			"deal: invalid blinds sb=%d bb=%d (need 0 < sb < bb)",
			smallBlind, bigBlind)
	}
	for _, p := range players {
		if p.Stack < bigBlind {
			return HandState{}, fmt.Errorf(
				"deal: seat %d stack %d below big blind %d (rebuy first)",
				p.Seat, p.Stack, bigBlind)
		}
	}

	// Sort players by seat for deterministic clockwise ordering.
	sorted := make([]SeatedPlayer, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Seat < sorted[j].Seat })

	// Locate dealer in sorted order.
	dealerIdx := -1
	for i, p := range sorted {
		if p.Seat == dealerSeat {
			dealerIdx = i
			break
		}
	}
	if dealerIdx < 0 {
		return HandState{}, fmt.Errorf(
			"deal: dealer seat %d is not seated", dealerSeat)
	}

	n := len(sorted)
	sbIdx := (dealerIdx + 1) % n
	bbIdx := (dealerIdx + 2) % n
	utgIdx := (dealerIdx + 3) % n

	// Snapshot the deck so we can persist it for replay/audit.
	deckSnapshot := deck.Snapshot()

	// Initialise PlayerStates with dealt hole cards.
	playerStates := make([]PlayerState, n)
	for i, p := range sorted {
		playerStates[i] = PlayerState{
			Seat:      p.Seat,
			Stack:     p.Stack,
			Status:    PlayerActive,
			HoleCards: nil,
		}
	}
	// One card at a time, starting left of dealer, for variant.HandSize rounds.
	// (Same as live dealing — order doesn't affect game outcome but matches
	// audit expectations.)
	for round := 0; round < variant.HandSize; round++ {
		for k := 0; k < n; k++ {
			idx := (sbIdx + k) % n
			c, err := deck.DealOne()
			if err != nil {
				return HandState{}, fmt.Errorf("deal hole: %w", err)
			}
			playerStates[idx].HoleCards = append(playerStates[idx].HoleCards, c)
		}
	}

	state := HandState{
		VariantKey:       variant.Key,
		Deck:             deckSnapshot,
		DeckPos:          deck.Pos(),
		Board:            nil,
		Players:          playerStates,
		Pot:              0,
		Phase:            PhasePreflop,
		DealerSeat:       sorted[dealerIdx].Seat,
		SmallBlindSeat:   sorted[sbIdx].Seat,
		BigBlindSeat:     sorted[bbIdx].Seat,
		CurrentActorSeat: sorted[utgIdx].Seat,
		CurrentBet:       0,
		SmallBlind:       smallBlind,
		BigBlind:         bigBlind,
		Actions:          nil,
		Winners:          nil,
	}

	// Post the blinds via the same mutation paths used for normal action,
	// so the action log is uniform.
	if err := postBlind(&state, sorted[sbIdx].Seat, smallBlind); err != nil {
		return HandState{}, fmt.Errorf("post sb: %w", err)
	}
	if err := postBlind(&state, sorted[bbIdx].Seat, bigBlind); err != nil {
		return HandState{}, fmt.Errorf("post bb: %w", err)
	}
	state.CurrentBet = bigBlind

	return state, nil
}

// postBlind mutates state to charge `amount` chips against the seat,
// move them to the pot, and record the action. Used only by Deal.
func postBlind(s *HandState, seat, amount int) error {
	idx, err := s.playerIndex(seat)
	if err != nil {
		return err
	}
	p := &s.Players[idx]
	if p.Stack < amount {
		// Defensive: Deal already validated stacks, so this should not happen.
		return fmt.Errorf("seat %d cannot post blind %d (stack %d)",
			seat, amount, p.Stack)
	}
	p.Stack -= amount
	s.Pot += amount
	s.Actions = append(s.Actions, Action{
		Sequence: len(s.Actions) + 1,
		Seat:     seat,
		Phase:    PhasePreflop,
		Type:     ActionPostBlind,
		Amount:   amount,
	})
	return nil
}

// playerIndex returns the index in s.Players for the given seat, or
// an error if no such seat is in the hand.
func (s *HandState) playerIndex(seat int) (int, error) {
	for i, p := range s.Players {
		if p.Seat == seat {
			return i, nil
		}
	}
	return -1, fmt.Errorf("seat %d not in hand", seat)
}

// activeSeatsClockwise returns seat numbers in sorted-clockwise order,
// starting from (and including) `from`. Skips folded and all-in seats.
func (s *HandState) activeSeatsClockwise(from int) []int {
	n := len(s.Players)
	startIdx := -1
	for i, p := range s.Players {
		if p.Seat == from {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return nil
	}
	out := make([]int, 0, n)
	for k := 0; k < n; k++ {
		idx := (startIdx + k) % n
		p := &s.Players[idx]
		if p.Status == PlayerActive {
			out = append(out, p.Seat)
		}
	}
	return out
}

// nextActiveSeatAfter returns the next seat clockwise from `seat` whose
// player is currently active, or -1 if no other active seats exist.
func (s *HandState) nextActiveSeatAfter(seat int) int {
	n := len(s.Players)
	startIdx := -1
	for i, p := range s.Players {
		if p.Seat == seat {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return -1
	}
	for k := 1; k <= n; k++ {
		idx := (startIdx + k) % n
		p := &s.Players[idx]
		if p.Status == PlayerActive {
			return p.Seat
		}
	}
	return -1
}

// firstActiveSeatClockwise returns the first active seat at or after
// `from`. -1 if none.
func (s *HandState) firstActiveSeatClockwise(from int) int {
	n := len(s.Players)
	startIdx := -1
	for i, p := range s.Players {
		if p.Seat == from {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		// from may not be seated (e.g., dealer who left). Find the first
		// active seat clockwise from where they would have been.
		// Fallback: just first active in sorted order.
		for i := range s.Players {
			if s.Players[i].Status == PlayerActive {
				return s.Players[i].Seat
			}
		}
		return -1
	}
	for k := 0; k < n; k++ {
		idx := (startIdx + k) % n
		p := &s.Players[idx]
		if p.Status == PlayerActive {
			return p.Seat
		}
	}
	return -1
}

// roundCommit returns how many chips `seat` has put into the pot during
// the current phase (sum of action amounts for that seat + phase).
func (s *HandState) roundCommit(seat int) int {
	sum := 0
	for _, a := range s.Actions {
		if a.Phase == s.Phase && a.Seat == seat {
			sum += a.Amount
		}
	}
	return sum
}

// hasActedVoluntarily reports whether `seat` has taken any non-blind
// action during the current phase.
func (s *HandState) hasActedVoluntarily(seat int) bool {
	for _, a := range s.Actions {
		if a.Phase == s.Phase && a.Seat == seat && a.Type != ActionPostBlind {
			return true
		}
	}
	return false
}

// countActive returns the number of seats still PlayerActive (can act).
func (s *HandState) countActive() int {
	n := 0
	for _, p := range s.Players {
		if p.Status == PlayerActive {
			n++
		}
	}
	return n
}

// countLive returns the number of seats not folded (Active + AllIn).
func (s *HandState) countLive() int {
	n := 0
	for _, p := range s.Players {
		if p.Status == PlayerActive || p.Status == PlayerAllIn {
			n++
		}
	}
	return n
}
