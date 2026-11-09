package service

import (
	"sync"
	"testing"
)

type recordingRoutingObserver struct {
	mu     sync.Mutex
	events []RoutingEvent
}

func (r *recordingRoutingObserver) OnRoutingEvent(ev RoutingEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

type panickingRoutingObserver struct{}

func (panickingRoutingObserver) OnRoutingEvent(RoutingEvent) { panic("boom") }

func TestEmitRoutingEventDeliversAndUnregisters(t *testing.T) {
	rec := &recordingRoutingObserver{}
	unregister := RegisterRoutingObserver(rec)

	EmitRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "r1"})
	if len(rec.events) != 1 || rec.events[0].RequestID != "r1" {
		t.Fatalf("expected one event for r1, got %+v", rec.events)
	}
	if rec.events[0].At.IsZero() {
		t.Fatalf("expected At to be filled")
	}

	unregister()
	EmitRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "r2"})
	if len(rec.events) != 1 {
		t.Fatalf("expected no delivery after unregister, got %d events", len(rec.events))
	}
}

func TestEmitRoutingEventSurvivesPanickingObserver(t *testing.T) {
	unregisterBad := RegisterRoutingObserver(panickingRoutingObserver{})
	defer unregisterBad()
	rec := &recordingRoutingObserver{}
	defer RegisterRoutingObserver(rec)()

	EmitRoutingEvent(RoutingEvent{Type: RoutingEventAttempt, RequestID: "r3"})
	if len(rec.events) != 1 {
		t.Fatalf("expected the healthy observer to still receive the event, got %d", len(rec.events))
	}
}

func TestRegisterRoutingObserverNilIsNoop(t *testing.T) {
	RegisterRoutingObserver(nil)()
}
