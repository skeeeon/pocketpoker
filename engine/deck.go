package engine

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
)

const DeckSize = 52

var ErrDeckExhausted = errors.New("deck exhausted")

// Deck is an ordered stack of cards. Cards are dealt from the front
// (index pos). The full underlying slice is preserved for replay/audit.
type Deck struct {
	cards []Card
	pos   int
}

// NewDeck returns a fresh 52-card deck in canonical order: by suit
// (clubs, diamonds, hearts, spades), each in ascending rank.
func NewDeck() *Deck {
	cards := make([]Card, 0, DeckSize)
	for s := Clubs; s <= Spades; s++ {
		for r := Two; r <= Ace; r++ {
			cards = append(cards, Card{Rank: r, Suit: s})
		}
	}
	return &Deck{cards: cards}
}

// Shuffle performs an in-place Fisher-Yates shuffle using the supplied
// random source. Pass crypto/rand.Reader in production.
func (d *Deck) Shuffle(r io.Reader) error {
	for i := len(d.cards) - 1; i > 0; i-- {
		j, err := uniformInt(r, i+1)
		if err != nil {
			return fmt.Errorf("shuffle: %w", err)
		}
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	}
	return nil
}

// NewShuffledDeck returns a freshly shuffled deck using crypto/rand.
func NewShuffledDeck() (*Deck, error) {
	d := NewDeck()
	if err := d.Shuffle(crand.Reader); err != nil {
		return nil, err
	}
	return d, nil
}

// Deal removes and returns the next n cards from the top of the deck.
func (d *Deck) Deal(n int) ([]Card, error) {
	if n < 0 {
		return nil, fmt.Errorf("deal: negative count %d", n)
	}
	if d.pos+n > len(d.cards) {
		return nil, ErrDeckExhausted
	}
	out := make([]Card, n)
	copy(out, d.cards[d.pos:d.pos+n])
	d.pos += n
	return out, nil
}

// DealOne is a convenience for Deal(1).
func (d *Deck) DealOne() (Card, error) {
	cards, err := d.Deal(1)
	if err != nil {
		return Card{}, err
	}
	return cards[0], nil
}

// Remaining returns how many cards are left to deal.
func (d *Deck) Remaining() int {
	return len(d.cards) - d.pos
}

// Snapshot returns a copy of the full underlying card sequence in
// dealing order. Useful for persisting deck_state for replay.
func (d *Deck) Snapshot() []Card {
	out := make([]Card, len(d.cards))
	copy(out, d.cards)
	return out
}

// Pos returns the current dealing position (0 = nothing dealt).
func (d *Deck) Pos() int {
	return d.pos
}

// DeckFromSnapshot rebuilds a deck from a previously snapshotted sequence
// with no cards dealt yet.
func DeckFromSnapshot(cards []Card) *Deck {
	out := make([]Card, len(cards))
	copy(out, cards)
	return &Deck{cards: out}
}

// uniformInt returns a uniform random integer in [0, n) drawn from r.
func uniformInt(r io.Reader, n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("uniformInt: non-positive bound %d", n)
	}
	v, err := crand.Int(r, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}
