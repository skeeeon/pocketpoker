package engine

import (
	"encoding/json"
	"testing"
)

func TestParseCardRoundTrip(t *testing.T) {
	for r := Two; r <= Ace; r++ {
		for s := Clubs; s <= Spades; s++ {
			want := Card{Rank: r, Suit: s}
			str := want.String()
			got, err := ParseCard(str)
			if err != nil {
				t.Fatalf("ParseCard(%q) err: %v", str, err)
			}
			if got != want {
				t.Fatalf("roundtrip: got %v want %v (str=%q)", got, want, str)
			}
		}
	}
}

func TestParseCardKnownStrings(t *testing.T) {
	cases := []struct {
		s    string
		want Card
	}{
		{"As", Card{Ace, Spades}},
		{"2c", Card{Two, Clubs}},
		{"Td", Card{Ten, Diamonds}},
		{"Kh", Card{King, Hearts}},
	}
	for _, tc := range cases {
		got, err := ParseCard(tc.s)
		if err != nil {
			t.Fatalf("ParseCard(%q): %v", tc.s, err)
		}
		if got != tc.want {
			t.Fatalf("ParseCard(%q)=%v want %v", tc.s, got, tc.want)
		}
	}
}

func TestParseCardErrors(t *testing.T) {
	bad := []string{"", "A", "Ass", "1s", "Ax", "as", "AS"}
	for _, b := range bad {
		if _, err := ParseCard(b); err == nil {
			t.Errorf("ParseCard(%q) expected error, got nil", b)
		}
	}
}

func TestParseCards(t *testing.T) {
	cards, err := ParseCards("As Kh 2c Td")
	if err != nil {
		t.Fatalf("ParseCards: %v", err)
	}
	if len(cards) != 4 {
		t.Fatalf("want 4 cards, got %d", len(cards))
	}
	want := []Card{{Ace, Spades}, {King, Hearts}, {Two, Clubs}, {Ten, Diamonds}}
	for i, c := range cards {
		if c != want[i] {
			t.Errorf("cards[%d]=%v want %v", i, c, want[i])
		}
	}
}

func TestCardJSON(t *testing.T) {
	c := Card{Ace, Spades}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"As"` {
		t.Fatalf("Marshal: got %s want %q", data, "As")
	}
	var out Card
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out != c {
		t.Fatalf("Unmarshal: got %v want %v", out, c)
	}
}

func TestCardJSONInSlice(t *testing.T) {
	cards := []Card{{Ace, Spades}, {Ten, Diamonds}}
	data, err := json.Marshal(cards)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `["As","Td"]` {
		t.Fatalf("Marshal slice: got %s", data)
	}
	var out []Card
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal slice: %v", err)
	}
	if len(out) != 2 || out[0] != cards[0] || out[1] != cards[1] {
		t.Fatalf("Unmarshal slice: got %v", out)
	}
}
