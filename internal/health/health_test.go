package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fixed(s State, detail string) Func {
	return func(context.Context) (State, string, string) { return s, detail, "" }
}

// The top-level state is the WORST of the checks, and `skipped` ranks below
// `ok` so a report where nothing could be answered does not read as healthy.
// Only `fail` clears `ready`: a warning is something a person fixes, and a
// probe can only restart or evict the process, which fixes neither.
func TestRollup(t *testing.T) {
	cases := []struct {
		name      string
		states    []State
		wantState State
		wantReady bool
	}{
		{"all ok", []State{StateOK, StateOK}, StateOK, true},
		{"a warning does not clear ready", []State{StateOK, StateWarn}, StateWarn, true},
		{"a failure clears ready", []State{StateOK, StateFail}, StateFail, false},
		{"fail outranks warn", []State{StateWarn, StateFail}, StateFail, false},
		{"skipped outranks ok", []State{StateOK, StateSkipped}, StateSkipped, true},
		{"warn outranks skipped", []State{StateSkipped, StateWarn}, StateWarn, true},
		{"all skipped is not healthy", []State{StateSkipped, StateSkipped}, StateSkipped, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProber("v0", time.Minute, time.Second)
			for i, s := range tc.states {
				p.Add(string(rune('a'+i)), fixed(s, "x"))
			}
			rep := p.Probe(context.Background())
			if rep.State != tc.wantState {
				t.Errorf("state = %q, want %q", rep.State, tc.wantState)
			}
			if rep.Ready != tc.wantReady {
				t.Errorf("ready = %v, want %v", rep.Ready, tc.wantReady)
			}
		})
	}
}

// Checks are reported sorted by name so two probes can be diffed line for line.
func TestChecksAreSortedByName(t *testing.T) {
	p := NewProber("v0", time.Minute, time.Second)
	for _, n := range []string{"zebra", "alpha", "monkey"} {
		p.Add(n, fixed(StateOK, "x"))
	}
	rep := p.Probe(context.Background())
	for i := 1; i < len(rep.Checks); i++ {
		if rep.Checks[i-1].Name > rep.Checks[i].Name {
			t.Fatalf("not sorted: %q before %q", rep.Checks[i-1].Name, rep.Checks[i].Name)
		}
	}
}

// A panicking check is a failure of that check and nothing more. Losing every
// other answer because one collector hit a nil map would be the wrong trade,
// and a probe endpoint that 500s tells an orchestrator nothing useful.
func TestPanickingCheckIsContained(t *testing.T) {
	p := NewProber("v0", time.Minute, time.Second)
	p.Add("boom", func(context.Context) (State, string, string) {
		panic(errors.New("nil map"))
	})
	p.Add("fine", fixed(StateOK, "x"))

	rep := p.Probe(context.Background())
	if len(rep.Checks) != 2 {
		t.Fatalf("got %d checks, want 2 — the other check was lost", len(rep.Checks))
	}
	var boom Check
	for _, c := range rep.Checks {
		if c.Name == "boom" {
			boom = c
		}
	}
	if boom.State != StateFail {
		t.Errorf("panicking check state = %q, want %q", boom.State, StateFail)
	}
	if rep.Ready {
		t.Error("a panicking check must clear ready")
	}
}

// Last() is nil until a probe completes, which is what makes the endpoint
// answer 503 rather than inventing an optimistic report.
func TestLastIsNilBeforeFirstProbe(t *testing.T) {
	p := NewProber("v0", time.Minute, time.Second)
	p.Add("x", fixed(StateOK, "x"))
	if p.Last() != nil {
		t.Fatal("Last() should be nil before the first probe")
	}
	p.Probe(context.Background())
	if p.Last() == nil {
		t.Fatal("Last() should be set after a probe")
	}
}

// OnProbe is how /metrics publishes the same numbers /api/ready serves. If it
// stopped firing, a scrape and a probe would disagree about one process.
func TestOnProbeObserversRun(t *testing.T) {
	p := NewProber("v0", time.Minute, time.Second)
	p.Add("x", fixed(StateWarn, "x"))

	var got *Report
	p.OnProbe(func(r *Report) { got = r })
	p.Probe(context.Background())

	if got == nil {
		t.Fatal("observer never ran")
	}
	if got.State != StateWarn {
		t.Errorf("observer saw state %q, want %q", got.State, StateWarn)
	}
}

// A check that ignores its context must not hold the loop forever.
func TestSlowCheckIsBounded(t *testing.T) {
	p := NewProber("v0", time.Minute, 50*time.Millisecond)
	p.Add("slow", func(ctx context.Context) (State, string, string) {
		<-ctx.Done()
		return StateFail, "timed out", ""
	})
	start := time.Now()
	p.Probe(context.Background())
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("probe took %v; the per-check timeout did not apply", elapsed)
	}
}
