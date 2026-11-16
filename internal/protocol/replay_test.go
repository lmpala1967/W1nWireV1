package protocol

import "testing"

func TestReplayWindowAcceptsNewCounters(t *testing.T) {
	var window ReplayWindow
	for counter := uint64(0); counter < 100; counter++ {
		if err := window.Accept(counter); err != nil {
			t.Fatalf("counter %d was rejected: %v", counter, err)
		}
	}
	if window.Top() != 99 {
		t.Errorf("top = %d, want 99", window.Top())
	}
}

func TestReplayWindowRejectsDuplicates(t *testing.T) {
	var window ReplayWindow
	if err := window.Accept(10); err != nil {
		t.Fatal(err)
	}
	if err := window.Accept(10); err == nil {
		t.Fatal("the same counter was accepted twice")
	}
	if err := window.Accept(11); err != nil {
		t.Fatalf("the next counter was rejected: %v", err)
	}
	if err := window.Accept(10); err == nil {
		t.Fatal("an old counter was accepted again")
	}
}

func TestReplayWindowRejectsOldCounters(t *testing.T) {
	var window ReplayWindow
	if err := window.Accept(1000); err != nil {
		t.Fatal(err)
	}
	if err := window.Accept(1000 - ReplayWindowSize); err == nil {
		t.Fatal("a counter outside of the window was accepted")
	}
}

func TestReplayWindowHandlesJumps(t *testing.T) {
	var window ReplayWindow
	if err := window.Accept(0); err != nil {
		t.Fatal(err)
	}
	if err := window.Accept(1000); err != nil {
		t.Fatal(err)
	}
	// The jump resets the window, the old counters are forgotten.
	if err := window.Accept(0); err == nil {
		t.Fatal("a counter from before the jump was accepted")
	}
	if err := window.Accept(1000); err == nil {
		t.Fatal("the current counter was accepted twice")
	}
}

func TestReplayWindowAcceptsCounterZeroTwice(t *testing.T) {
	var window ReplayWindow
	if err := window.Accept(0); err != nil {
		t.Fatal(err)
	}
	if err := window.Accept(0); err == nil {
		t.Fatal("counter zero was accepted twice")
	}
}
