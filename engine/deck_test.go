package engine

import (
	crand "crypto/rand"
	"testing"
)

func TestNewDeckHas52UniqueCards(t *testing.T) {
	d := NewDeck()
	if d.Remaining() != DeckSize {
		t.Fatalf("Remaining()=%d want %d", d.Remaining(), DeckSize)
	}
	seen := make(map[Card]bool, DeckSize)
	for _, c := range d.Snapshot() {
		if seen[c] {
			t.Fatalf("duplicate card %v", c)
		}
		seen[c] = true
	}
	if len(seen) != DeckSize {
		t.Fatalf("got %d unique cards want %d", len(seen), DeckSize)
	}
}

func TestDealAdvancesPosAndReturnsCards(t *testing.T) {
	d := NewDeck()
	first, err := d.Deal(5)
	if err != nil {
		t.Fatalf("Deal(5): %v", err)
	}
	if len(first) != 5 {
		t.Fatalf("got %d cards want 5", len(first))
	}
	if d.Pos() != 5 {
		t.Fatalf("Pos()=%d want 5", d.Pos())
	}
	if d.Remaining() != DeckSize-5 {
		t.Fatalf("Remaining()=%d want %d", d.Remaining(), DeckSize-5)
	}

	second, err := d.Deal(3)
	if err != nil {
		t.Fatalf("Deal(3): %v", err)
	}
	// First two batches must be disjoint.
	for _, a := range first {
		for _, b := range second {
			if a == b {
				t.Fatalf("Deal returned duplicate %v", a)
			}
		}
	}
}

func TestDealOne(t *testing.T) {
	d := NewDeck()
	c, err := d.DealOne()
	if err != nil {
		t.Fatalf("DealOne: %v", err)
	}
	// Canonical order is 2c first.
	want := Card{Two, Clubs}
	if c != want {
		t.Fatalf("DealOne()=%v want %v", c, want)
	}
}

func TestDealExhaustionError(t *testing.T) {
	d := NewDeck()
	if _, err := d.Deal(DeckSize); err != nil {
		t.Fatalf("Deal(52): %v", err)
	}
	if _, err := d.Deal(1); err != ErrDeckExhausted {
		t.Fatalf("Deal past end: err=%v want ErrDeckExhausted", err)
	}
}

func TestShuffleProducesPermutation(t *testing.T) {
	d := NewDeck()
	if err := d.Shuffle(crand.Reader); err != nil {
		t.Fatalf("Shuffle: %v", err)
	}
	if d.Remaining() != DeckSize {
		t.Fatalf("Remaining after shuffle=%d", d.Remaining())
	}
	seen := make(map[Card]bool, DeckSize)
	for _, c := range d.Snapshot() {
		if seen[c] {
			t.Fatalf("duplicate after shuffle: %v", c)
		}
		seen[c] = true
	}
	if len(seen) != DeckSize {
		t.Fatalf("after shuffle got %d unique cards", len(seen))
	}
}

func TestShuffleActuallyShuffles(t *testing.T) {
	// Statistical sanity: a shuffle should disturb canonical order. Check
	// that at least 30 of 52 positions changed. P(false fail) is astronomical.
	d := NewDeck()
	original := d.Snapshot()
	if err := d.Shuffle(crand.Reader); err != nil {
		t.Fatalf("Shuffle: %v", err)
	}
	shuffled := d.Snapshot()
	moved := 0
	for i := range original {
		if original[i] != shuffled[i] {
			moved++
		}
	}
	if moved < 30 {
		t.Fatalf("only %d/52 cards moved; shuffle looks broken", moved)
	}
}

func TestSnapshotIsDeepCopy(t *testing.T) {
	d := NewDeck()
	snap := d.Snapshot()
	snap[0] = Card{Ace, Spades}
	if got := d.Snapshot()[0]; got == (Card{Ace, Spades}) {
		t.Fatalf("Snapshot mutation leaked back into deck: %v", got)
	}
}

func TestDeckFromSnapshotPreservesOrder(t *testing.T) {
	original := MustParseCards("As Kh 2c Td 5s 9h Jc 4d")
	d := DeckFromSnapshot(original)
	for i, want := range original {
		got, err := d.DealOne()
		if err != nil {
			t.Fatalf("DealOne[%d]: %v", i, err)
		}
		if got != want {
			t.Fatalf("dealt[%d]=%v want %v", i, got, want)
		}
	}
}

func TestDeckFromSnapshotIndependentOfInput(t *testing.T) {
	original := MustParseCards("As Kh 2c")
	d := DeckFromSnapshot(original)
	original[0] = Card{Two, Clubs}
	first, _ := d.DealOne()
	if first != (Card{Ace, Spades}) {
		t.Fatalf("DeckFromSnapshot did not copy input: dealt %v", first)
	}
}
