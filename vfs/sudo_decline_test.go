//go:build !windows

package vfs

import (
	"errors"
	"testing"
	"time"
)

// A cancelled password dialog silences the next ones for a while, so that a
// scan over many closed folders asks once (f4#1998).
func TestConnectRefusesAtOnceAfterDeclinedPassword(t *testing.T) {
	c := &SudoClient{}
	c.markDeclined()
	start := time.Now()
	err := c.Connect()
	if !errors.Is(err, ErrElevationDeclined) {
		t.Fatalf("Connect after a cancelled dialog = %v, want ErrElevationDeclined", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Connect took %v, it must not start sudo again", d)
	}
}

func TestDeclinedPasswordIsForgottenAfterHold(t *testing.T) {
	old := ElevationDeclineHold
	ElevationDeclineHold = 10 * time.Millisecond
	t.Cleanup(func() { ElevationDeclineHold = old })
	c := &SudoClient{}
	c.markDeclined()
	time.Sleep(30 * time.Millisecond)
	if c.recentlyDeclined() {
		t.Fatal("the refusal outlived ElevationDeclineHold")
	}
}
