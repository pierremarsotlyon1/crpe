//go:build integration

package storebilling

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

type testStore struct {
	mu               sync.Mutex
	values           map[string]storeprovider.Snapshot
	acknowledgements int
	failAck          bool
}

func (p *testStore) Verify(_ context.Context, ref string) (storeprovider.Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.values[ref]
	if !ok {
		return v, storeprovider.ErrInvalid
	}
	return v, nil
}
func (p *testStore) put(ref string, v storeprovider.Snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values[ref] = v
}
func (p *testStore) Acknowledge(_ context.Context, _ storeprovider.Snapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failAck {
		return storeprovider.ErrUnavailable
	}
	p.acknowledgements++
	return nil
}
func (p *testStore) Notification(_ context.Context, h http.Header, body []byte) (storeprovider.Notification, error) {
	if h.Get("X-Test-Verified") != "yes" {
		return storeprovider.Notification{}, storeprovider.ErrInvalid
	}
	return storeprovider.Notification{Reference: string(body), EventID: h.Get("X-Test-Event")}, nil
}

const fixturePlans = `[{"platform":"google","tier":"suivi","planKey":"suivi","productId":"practice","basePlanId":"month"},{"platform":"google","tier":"suivi","planKey":"suivi-year","productId":"practice","basePlanId":"year"},{"platform":"apple","tier":"autonomie","planKey":"base","productId":"oral.base"}]`

func setup(t *testing.T) (*Service, *testStore, string, string) {
	t.Helper()
	db := testdatabase.Open(t)
	p := &testStore{values: map[string]storeprovider.Snapshot{}}
	env := map[string]string{"STORE_PRODUCTS_JSON": fixturePlans, "APPLE_STORE_ENVIRONMENT": "Sandbox", "GOOGLE_PLAY_ENVIRONMENT": "Sandbox", "STORE_REFERENCE_KEY": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))}
	s, e := New(db, func(k string) string { return env[k] }, map[string]storeprovider.Provider{"google": p, "apple": p})
	if e != nil {
		t.Fatal(e)
	}
	insert := func(email string) string {
		var id string
		if e := db.QueryRow(t.Context(), `INSERT INTO users(email,role) VALUES($1,'student') RETURNING id::text`, email).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	return s, p, insert("a@example.test"), insert("b@example.test")
}
func playSnapshot(owner, ref string) storeprovider.Snapshot {
	return storeprovider.Snapshot{Reference: ref, OriginalReference: ref, AccountToken: accountToken("google", owner), ProductID: "practice", BasePlanID: "month", Environment: "Sandbox", State: "active", ExpiresAt: time.Now().Add(time.Hour), ObservedAt: time.Now(), NeedsAcknowledgement: true}
}
func TestStoreReplayOwnershipAndDurableAcknowledgement(t *testing.T) {
	s, p, a, b := setup(t)
	ctx := t.Context()
	v := playSnapshot(a, "play-secret-original")
	p.put(v.Reference, v)
	if _, e := s.Purchase(ctx, b, "google", "practice", v.Reference); !errors.Is(e, ErrOwner) {
		t.Fatal(e)
	}
	pending := v
	pending.State = "pending"
	p.put(v.Reference, pending)
	if r, e := s.Purchase(ctx, a, "google", "practice", v.Reference); e != nil || r.Status != "pending" {
		t.Fatal(r, e)
	}
	rights, e := Active(ctx, s.db, a, nil)
	if e != nil || len(rights) != 0 {
		t.Fatal(rights, e)
	}
	p.put(v.Reference, v)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			r, e := s.Purchase(ctx, a, "google", "practice", v.Reference)
			if e != nil || r.Status != "active" {
				t.Error(r, e)
			}
		})
	}
	wg.Wait()
	var cipher []byte
	var rowID string
	if e = s.db.QueryRow(ctx, "SELECT id::text,lookup_cipher FROM store_memberships WHERE user_id=$1", a).Scan(&rowID, &cipher); e != nil || bytes.Contains(cipher, []byte(v.Reference)) {
		t.Fatal(e)
	}
	var count int
	if e = s.db.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind='ack_store_purchase'").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	payload, _ := json.Marshal(map[string]string{"id": rowID})
	msg := worker.Message{Payload: payload}
	h := s.AcknowledgeHandler()
	p.failAck = true
	if e = h(ctx, msg); !errors.Is(e, storeprovider.ErrUnavailable) {
		t.Fatal(e)
	}
	p.failAck = false
	for range 2 {
		if e = h(ctx, msg); e != nil {
			t.Fatal(e)
		}
	}
	if p.acknowledgements != 1 {
		t.Fatal(p.acknowledgements)
	}
	hds := http.Header{"X-Test-Event": {"refund"}, "X-Test-Verified": {"yes"}}
	v.State = "revoked"
	v.ObservedAt = v.ObservedAt.Add(-time.Hour)
	p.put(v.Reference, v)
	if e := s.Notify(ctx, "google", nil, []byte(v.Reference)); !errors.Is(e, storeprovider.ErrInvalid) {
		t.Fatal(e)
	}
	for range 2 {
		if e = s.Notify(ctx, "google", hds, []byte(v.Reference)); e != nil {
			t.Fatal(e)
		}
	}
	rights, e = Active(ctx, s.db, a, nil)
	if e != nil || len(rights) != 0 {
		t.Fatal("refund despite observation clock skew", rights, e)
	}
	v.State = "active"
	v.Environment = "Production"
	p.put(v.Reference, v)
	if _, e = s.Purchase(ctx, a, "google", "practice", v.Reference); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestReplacementOwnershipAndDeletedAccountTombstones(t *testing.T) {
	s, p, a, b := setup(t)
	ctx := t.Context()
	parent := playSnapshot(a, "parent")
	p.put("parent", parent)
	if _, e := s.Purchase(ctx, a, "google", "practice", "parent"); e != nil {
		t.Fatal(e)
	}
	child := playSnapshot(b, "child")
	child.LinkedReference = "parent"
	p.put("child", child)
	if _, e := s.Purchase(ctx, b, "google", "practice", "child"); !errors.Is(e, ErrOwner) {
		t.Fatal(e)
	}
	h := http.Header{"X-Test-Event": {"wrong-owner"}, "X-Test-Verified": {"yes"}}
	if e := s.Notify(ctx, "google", h, []byte("child")); e != nil {
		t.Fatal(e)
	}
	rights, e := Active(ctx, s.db, b, nil)
	if e != nil || len(rights) != 0 {
		t.Fatal("foreign notification savepoint", rights, e)
	}
	child.AccountToken = accountToken("google", a)
	child.State = "pending"
	p.put("child", child)
	if _, e = s.Purchase(ctx, a, "google", "practice", "child"); e != nil {
		t.Fatal(e)
	}
	rights, e = Active(ctx, s.db, a, nil)
	if e != nil || len(rights) != 1 {
		t.Fatal(rights, e)
	}
	child.State = "active"
	child.BasePlanID = "year"
	p.put("child", child)
	if _, e = s.Purchase(ctx, a, "google", "practice", "child"); e != nil {
		t.Fatal(e)
	}
	if r, e := s.Purchase(ctx, a, "google", "practice", "parent"); e != nil || r.Status != "expired" {
		t.Fatal(r, e)
	}
	rights, e = Active(ctx, s.db, a, nil)
	if e != nil || len(rights) != 1 || rights[0].PlanKey != "suivi-year" {
		t.Fatal(rights, e)
	}
	if _, e = s.db.Exec(ctx, "DELETE FROM users WHERE id=$1", a); e != nil {
		t.Fatal(e)
	}
	child.AccountToken = accountToken("google", b)
	p.put("child", child)
	if _, e = s.Purchase(ctx, b, "google", "practice", "child"); !errors.Is(e, ErrOwner) {
		t.Fatal("cannot claim deleted owner", e)
	}
}
func TestStoreOrderingAndSalesOffRestore(t *testing.T) {
	s, p, a, _ := setup(t)
	ctx := t.Context()
	apple := storeprovider.Snapshot{Reference: "222", OriginalReference: "111", AccountToken: a, Environment: "Sandbox", ProductID: "oral.base", State: "active", ObservedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	p.put("222", apple)
	p.put("111", apple)
	cat, e := s.Catalog(ctx, a, "google", "https://site.example", true, nil)
	if e != nil || !cat.Enabled || len(cat.Items) != 2 || cat.AccountToken != accountToken("google", a) {
		t.Fatal(cat, e)
	}
	if _, e = s.Reserve(ctx, a, OrderInput{Platform: "google", ProductID: "practice", BasePlanID: "month", ClientID: ""}, false, nil); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
	order, e := s.Reserve(ctx, a, OrderInput{Platform: "google", ProductID: "practice", BasePlanID: "month", ClientID: ""}, true, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, a, OrderInput{Platform: "apple", ProductID: "oral.base", BasePlanID: "", ClientID: ""}, true, nil); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e = s.Cancel(ctx, a, "google", order.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, a, OrderInput{Platform: "apple", ProductID: "oral.base", BasePlanID: "", ClientID: ""}, true, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(ctx, "UPDATE store_orders SET expires_at=now()-interval '1 minute' WHERE user_id=$1", a); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, a, OrderInput{Platform: "google", ProductID: "practice", BasePlanID: "year", ClientID: ""}, true, nil); e != nil {
		t.Fatal(e)
	}
	if r, e := s.Purchase(ctx, a, "apple", "previous-product", "222"); e != nil || r.Status != "active" {
		t.Fatal(r, e)
	}
	for _, sales := range []bool{true, false} {
		cat, e = s.Catalog(ctx, a, "google", "https://site.example", sales, nil)
		if e != nil || cat.Enabled || cat.ActiveProvider != "apple" || len(cat.Items) != 0 {
			t.Fatal(cat, e)
		}
	}
	if _, e = s.Reserve(ctx, a, OrderInput{Platform: "google", ProductID: "practice", BasePlanID: "month", ClientID: ""}, true, nil); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}

func TestAckReconcilesLatestBenefitBeforeAcknowledging(t *testing.T) {
	s, p, a, _ := setup(t)
	ctx := t.Context()
	for _, state := range []string{"revoked", "expired", "active"} {
		v := playSnapshot(a, "ack-"+state)
		p.put(v.Reference, v)
		if _, e := s.Purchase(ctx, a, "google", "practice", v.Reference); e != nil {
			t.Fatal(e)
		}
		var id string
		if e := s.db.QueryRow(ctx, "SELECT id::text FROM store_memberships WHERE original_hash=$1", digest(v.Reference)).Scan(&id); e != nil {
			t.Fatal(e)
		}
		msg, _ := json.Marshal(map[string]string{"id": id})
		v.State = state
		v.BasePlanID = "year"
		p.put(v.Reference, v)
		if e := s.AcknowledgeHandler()(ctx, worker.Message{Payload: msg}); e != nil {
			t.Fatal(e)
		}
		var got, plan string
		var ack bool
		if e := s.db.QueryRow(ctx, "SELECT state,plan_key,acknowledged FROM store_memberships WHERE id=$1", id).Scan(&got, &plan, &ack); e != nil || got != state || plan != "suivi-year" || ack != (state == "active") {
			t.Fatal(got, plan, ack, e)
		}
	}
	if p.acknowledgements != 1 {
		t.Fatal("only active purchases acknowledged", p.acknowledgements)
	}
	v := playSnapshot(a, "ack-active")
	v.AccountToken = ""
	v.State = "expired"
	p.put(v.Reference, v)
	if e := s.Notify(ctx, "google", http.Header{"X-Test-Event": {"unlinked-expiry"}, "X-Test-Verified": {"yes"}}, []byte(v.Reference)); e != nil {
		t.Fatal(e)
	}
	if rights, e := Active(ctx, s.db, a, nil); e != nil || len(rights) != 0 {
		t.Fatal(rights, e)
	}
}

func TestCheckoutNonceAndStalePlanAcrossDevices(t *testing.T) {
	s, p, user, _ := setup(t)
	ctx := t.Context()
	first := OrderInput{Platform: "google", ProductID: "practice", BasePlanID: "month", ClientID: "B964A6A0-E82E-4B12-AF87-4E82258D2C3F"}
	second := OrderInput{Platform: "google", ProductID: "practice", BasePlanID: "year", ClientID: "e591b92c-027f-4a31-bb53-b5887cedb997"}
	a, e := s.Reserve(ctx, user, first, true, nil)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Reserve(ctx, user, first, true, nil)
	if e != nil || again.ID != a.ID {
		t.Fatal(again, e)
	}
	if _, e = s.Reserve(ctx, user, second, true, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("concurrent device", e)
	}
	if e = s.Cancel(ctx, user, "google", a.ID); e != nil {
		t.Fatal(e)
	}
	b, e := s.Reserve(ctx, user, second, true, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Cancel(ctx, user, "google", a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, user, OrderInput{Platform: "apple", ProductID: "oral.base"}, true, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("stale cancellation released a different order", e)
	}
	v := playSnapshot(user, "purchased")
	p.put(v.Reference, v)
	if _, e = s.Purchase(ctx, user, "google", "practice", v.Reference); e != nil {
		t.Fatal(e)
	}
	var current string
	if e = s.db.QueryRow(ctx, "SELECT id::text FROM store_orders WHERE user_id=$1", user).Scan(&current); e != nil || current != b.ID {
		t.Fatal("restore released order", e, current)
	}
	if e = s.Cancel(ctx, user, "google", b.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, user, second, true, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("stale empty catalogue", e)
	}
	second.ExpectedActiveProductID = "practice"
	second.ExpectedActiveBasePlanID = "month"
	if _, e = s.Reserve(ctx, user, second, true, nil); e != nil {
		t.Fatal("confirmed replacement", e)
	}
}
