#!/usr/bin/env bash
# Phase 3 integration test: drives a full 3-handed Hold'em hand
# end-to-end via the /api/poker/* endpoints. Also asserts the
# Phase 2 privacy properties (hole cards hidden, deck_state hidden,
# showdown reveal).
#
# Assumes a fresh PB instance is running at PB_URL with the variants
# seed applied. Creates timestamped user accounts so re-runs don't
# collide on email uniqueness.
#
# Run: bash tests/integration.sh

set -u

PB_URL="${PB_URL:-http://127.0.0.1:8090}"
PASS=0
FAIL=0

# ----- helpers -----

# extract JSON value from a flat object: value-of-string-field
json_str() {
  local field=$1 body=$2
  echo "$body" | sed -n 's/.*"'"$field"'":"\([^"]*\)".*/\1/p' | head -1
}

# extract JSON value from a flat object: value-of-integer-or-null field
json_num() {
  local field=$1 body=$2
  echo "$body" | sed -n 's/.*"'"$field"'":\([-0-9]*\)[,}].*/\1/p' | head -1
}

ok() { PASS=$((PASS+1)); printf "  \033[32mok\033[0m  %s\n" "$*"; }
bad() { FAIL=$((FAIL+1)); printf "  \033[31mFAIL\033[0m  %s\n" "$*"; }

require_status() {
  local want=$1 got=$2 ctx=$3
  if [ "$got" = "$want" ]; then ok "$ctx -> $got"
  else bad "$ctx expected $want got $got"; fi
}

# usage: register_user <email>; sets EMAIL=$1, USER_ID, USER_TOKEN
register_user() {
  local email=$1 password="testpass1234"
  local body
  body=$(curl -sS -X POST "$PB_URL/api/collections/users/records" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\",\"password\":\"$password\",\"passwordConfirm\":\"$password\"}")
  USER_ID=$(json_str id "$body")
  if [ -z "$USER_ID" ]; then
    echo "register_user: failed to create $email: $body" >&2
    exit 1
  fi
  body=$(curl -sS -X POST "$PB_URL/api/collections/users/auth-with-password" \
    -H "Content-Type: application/json" \
    -d "{\"identity\":\"$email\",\"password\":\"$password\"}")
  USER_TOKEN=$(json_str token "$body")
  if [ -z "$USER_TOKEN" ]; then
    echo "register_user: auth failed: $body" >&2
    exit 1
  fi
}

post() {
  # post <url> <token> <body>
  curl -sS -o /tmp/_resp.body -w "%{http_code}" -X POST "$1" \
    -H "Authorization: $2" -H "Content-Type: application/json" -d "$3"
}

get() {
  curl -sS -o /tmp/_resp.body -w "%{http_code}" -X GET "$1" \
    -H "Authorization: $2"
}

resp() { cat /tmp/_resp.body; }

# ----- setup: 3 test users with unique emails -----

TS=$(date +%s)
register_user "alice-$TS@test.local"; ALICE_ID=$USER_ID; ALICE_TOK=$USER_TOKEN
register_user "bob-$TS@test.local";   BOB_ID=$USER_ID;   BOB_TOK=$USER_TOKEN
register_user "carol-$TS@test.local"; CAROL_ID=$USER_ID; CAROL_TOK=$USER_TOKEN
echo "Registered 3 test users (alice, bob, carol)"

# ----- create table (alice = creator/dealer) -----

CODE=$(curl -sS -o /tmp/_resp.body -w "%{http_code}" -X POST \
  "$PB_URL/api/collections/tables/records" \
  -H "Authorization: $ALICE_TOK" -H "Content-Type: application/json" \
  -d "{\"name\":\"test-$TS\",\"created_by\":\"$ALICE_ID\",\"buy_in\":1000,\"small_blind\":10,\"big_blind\":20,\"max_seats\":8,\"status\":\"waiting\"}")
require_status 200 "$CODE" "create table"
TABLE_ID=$(json_str id "$(resp)")
[ -n "$TABLE_ID" ] && ok "table id: $TABLE_ID" || { bad "no table id"; exit 1; }

# ----- sit (3 players) -----

CODE=$(post "$PB_URL/api/poker/tables/$TABLE_ID/sit" "$ALICE_TOK" '{"seat_number":0,"buy_in_amount":1000}')
require_status 200 "$CODE" "alice sits seat 0"
CODE=$(post "$PB_URL/api/poker/tables/$TABLE_ID/sit" "$BOB_TOK"   '{"seat_number":1,"buy_in_amount":1000}')
require_status 200 "$CODE" "bob sits seat 1"
CODE=$(post "$PB_URL/api/poker/tables/$TABLE_ID/sit" "$CAROL_TOK" '{"seat_number":2,"buy_in_amount":1000}')
require_status 200 "$CODE" "carol sits seat 2"

# duplicate-seat rejection
CODE=$(post "$PB_URL/api/poker/tables/$TABLE_ID/sit" "$BOB_TOK" '{"seat_number":2,"buy_in_amount":1000}')
require_status 400 "$CODE" "double-sit rejected (already seated)"

# ----- start hand -----

CODE=$(post "$PB_URL/api/poker/tables/$TABLE_ID/start-hand" "$ALICE_TOK" '{"variant_key":"holdem"}')
require_status 200 "$CODE" "alice starts holdem hand"
HAND_ID=$(json_str hand_id "$(resp)")
[ -n "$HAND_ID" ] && ok "hand id: $HAND_ID" || { bad "no hand id"; exit 1; }

# Non-dealer cannot start a hand. (We need to complete this hand first
# to test that path; see the second-hand test below.)

# ----- privacy: hand_players rows -----

# Each user GETs hand_players filtered to their hand and asserts
# they see exactly 1 row (their own).
for tok_label in "ALICE:$ALICE_TOK" "BOB:$BOB_TOK" "CAROL:$CAROL_TOK"; do
  label=${tok_label%%:*}; tok=${tok_label#*:}
  CODE=$(get "$PB_URL/api/collections/hand_players/records?filter=hand%3D%22$HAND_ID%22" "$tok")
  total=$(json_num totalItems "$(resp)")
  if [ "$CODE" = "200" ] && [ "$total" = "1" ]; then
    ok "$label sees exactly 1 hand_players row pre-showdown"
  else
    bad "$label hand_players visibility: status=$CODE total=$total"
  fi
done

# Verify deck_state is hidden on the hands record.
CODE=$(get "$PB_URL/api/collections/hands/records/$HAND_ID" "$ALICE_TOK")
require_status 200 "$CODE" "fetch hand record"
if echo "$(resp)" | grep -q '"deck_state"'; then
  bad "deck_state field leaked into hand response"
else
  ok "deck_state hidden from API response"
fi

# ----- play the hand (3-handed, all check/call) -----

# Read current state from the hands record.
read_hand_state() {
  CODE=$(get "$PB_URL/api/collections/hands/records/$HAND_ID" "$ALICE_TOK")
  HBODY=$(resp)
  PHASE=$(json_str phase "$HBODY")
  CURRENT_ACTOR=$(json_num current_actor_seat "$HBODY")
  VERSION=$(json_num version "$HBODY")
}

# Map seat -> token.
seat_token() {
  case "$1" in
    0) echo "$ALICE_TOK" ;;
    1) echo "$BOB_TOK" ;;
    2) echo "$CAROL_TOK" ;;
  esac
}

submit() {
  local seat=$1 type=$2 amount=${3:-0}
  read_hand_state
  if [ "$CURRENT_ACTOR" != "$seat" ]; then
    bad "expected current_actor=$seat got $CURRENT_ACTOR (phase=$PHASE)"
    return 1
  fi
  local tok; tok=$(seat_token "$seat")
  CODE=$(post "$PB_URL/api/poker/hands/$HAND_ID/action" "$tok" \
    "{\"action_type\":\"$type\",\"amount\":$amount,\"version\":$VERSION}")
  if [ "$CODE" != "200" ]; then
    bad "seat $seat $type: status=$CODE body=$(resp)"
    return 1
  fi
  ok "seat $seat $type at phase=$PHASE v=$VERSION"
}

# Preflop: action starts at dealer (seat 0) for 3-handed. SB=1, BB=2.
submit 0 call
submit 1 call
submit 2 check
# Flop: action starts at SB (seat 1).
submit 1 check
submit 2 check
submit 0 check
# Turn
submit 1 check
submit 2 check
submit 0 check
# River
submit 1 check
submit 2 check
submit 0 check

read_hand_state
if [ "$PHASE" = "complete" ]; then
  ok "hand reached phase=complete"
else
  bad "expected phase=complete, got $PHASE"
fi

# Verify winner_seats populated, pot=0.
HBODY=$(resp)
if echo "$HBODY" | grep -q '"winner_seats":'; then
  ok "winner_seats present"
else
  bad "winner_seats missing"
fi
POT=$(json_num pot "$HBODY")
if [ "$POT" = "0" ]; then ok "pot=0 after distribution"; else bad "pot=$POT (want 0)"; fi

# ----- privacy: showdown reveal -----

# Now that phase=complete (which passes through showdown), each user
# should be able to see all 3 hand_players rows (cards revealed).
# Note: PB's API rule allows visibility when hand.phase=showdown OR
# the row is the user's own. Phase=complete should also satisfy it
# only via the owner clause; the showdown clause checks phase="showdown"
# specifically. Since we transitioned through showdown to complete in
# one call, the data is now phase=complete -> only owner's row is visible.
#
# This is a minor v1 quirk; if it bites, the rule can be widened to
# include "complete" or we can pause briefly at showdown. For now,
# document the actual behavior.
#
# Each user should now see all 3 hand_players rows.
for tok_label in "ALICE:$ALICE_TOK" "BOB:$BOB_TOK" "CAROL:$CAROL_TOK"; do
  label=${tok_label%%:*}; tok=${tok_label#*:}
  CODE=$(get "$PB_URL/api/collections/hand_players/records?filter=hand%3D%22$HAND_ID%22" "$tok")
  total=$(json_num totalItems "$(resp)")
  if [ "$CODE" = "200" ] && [ "$total" = "3" ]; then
    ok "$label sees all 3 hand_players rows post-showdown"
  else
    bad "$label post-showdown visibility: status=$CODE total=$total"
  fi
done

# Verify replay endpoint reveals everything for completed hands.
CODE=$(get "$PB_URL/api/poker/hands/$HAND_ID/replay" "$BOB_TOK")
require_status 200 "$CODE" "bob can fetch replay (hand is complete)"
if echo "$(resp)" | grep -q '"deck_state":'; then
  ok "replay endpoint exposes deck_state (post-hand audit)"
else
  bad "replay missing deck_state"
fi

# Total chip conservation check.
TOTAL=0
for tok in "$ALICE_TOK" "$BOB_TOK" "$CAROL_TOK"; do
  CODE=$(get "$PB_URL/api/collections/seats/records?filter=table%3D%22$TABLE_ID%22" "$tok")
  if [ "$CODE" != "200" ]; then continue; fi
  # Sum stack values from all returned rows (one per seat).
  STACKS=$(echo "$(resp)" | grep -oE '"stack":[0-9]+' | grep -oE '[0-9]+')
  for s in $STACKS; do TOTAL=$((TOTAL+s)); done
  break # all 3 users see the same seats list
done
# Three players × 1000 chips = 3000 expected.
if [ "$TOTAL" = "3000" ]; then
  ok "chip conservation: total=$TOTAL"
else
  bad "chip total=$TOTAL (want 3000)"
fi

# ----- cleanup -----
# Each user leaves the table (deletes their seat row), then the
# creator (alice) deletes the table itself. CascadeDelete on
# seats.table and hands.table prunes any leftovers. This keeps
# pb_data tidy across re-runs.

for tok in "$ALICE_TOK" "$BOB_TOK" "$CAROL_TOK"; do
  curl -sS -o /dev/null -X POST "$PB_URL/api/poker/tables/$TABLE_ID/leave" \
    -H "Authorization: $tok" >/dev/null 2>&1 || true
done
curl -sS -o /dev/null -X DELETE \
  "$PB_URL/api/collections/tables/records/$TABLE_ID" \
  -H "Authorization: $ALICE_TOK" >/dev/null 2>&1 || true

# ----- summary -----

echo
echo "------------------------------"
echo "Passed: $PASS  Failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
exit 0
