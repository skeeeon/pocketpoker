package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

const suitChars = "cdhs"

func (s Suit) String() string {
	if int(s) >= len(suitChars) {
		return "?"
	}
	return string(suitChars[s])
}

type Rank uint8

const (
	Two Rank = 2 + iota
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

const rankChars = "23456789TJQKA"

func (r Rank) String() string {
	if r < Two || r > Ace {
		return "?"
	}
	return string(rankChars[r-Two])
}

type Card struct {
	Rank Rank
	Suit Suit
}

func (c Card) String() string {
	return c.Rank.String() + c.Suit.String()
}

func ParseCard(s string) (Card, error) {
	if len(s) != 2 {
		return Card{}, fmt.Errorf("invalid card %q: must be 2 chars", s)
	}
	rIdx := strings.IndexByte(rankChars, s[0])
	if rIdx < 0 {
		return Card{}, fmt.Errorf("invalid rank %q in card %q", s[:1], s)
	}
	sIdx := strings.IndexByte(suitChars, s[1])
	if sIdx < 0 {
		return Card{}, fmt.Errorf("invalid suit %q in card %q", s[1:], s)
	}
	return Card{Rank: Rank(rIdx) + Two, Suit: Suit(sIdx)}, nil
}

func MustParseCard(s string) Card {
	c, err := ParseCard(s)
	if err != nil {
		panic(err)
	}
	return c
}

func ParseCards(s string) ([]Card, error) {
	fields := strings.Fields(s)
	out := make([]Card, len(fields))
	for i, f := range fields {
		c, err := ParseCard(f)
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

func MustParseCards(s string) []Card {
	cards, err := ParseCards(s)
	if err != nil {
		panic(err)
	}
	return cards
}

func (c Card) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *Card) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseCard(s)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}
