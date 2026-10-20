package hub

import "testing"

func TestAudience(t *testing.T) {
	h := New()
	a, _, _ := h.Subscribe("laptop", "id-laptop")
	b, _, _ := h.Subscribe("phone", "id-phone")
	h.Publish(Event{Type: "message", Audience: []string{"id-laptop"}})
	h.Publish(Event{Type: "presence"})
	if e := <-a; e.Type != "message" {
		t.Fatalf("laptop got %q first", e.Type)
	}
	if e := <-b; e.Type != "presence" {
		t.Fatalf("phone got %q, want only the public event", e.Type)
	}
	if len(b) != 0 {
		t.Fatalf("phone has %d extra events", len(b))
	}
}
