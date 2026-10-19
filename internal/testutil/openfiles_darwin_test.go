package testutil

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// Resolving a name makes libSystem connect to mDNSResponder and keep that
// connection, with a kernel control socket, for the rest of the process;
// neither is the tests' and neither is listed.
func TestOpenFilesLeavesOutTheResolverSockets(t *testing.T) {
	before := openFiles()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = net.DefaultResolver.LookupHost(ctx, "localhost") // The lookup itself may fail; what it opens is the point.

	for _, f := range leakedFiles(before, openFiles()) {
		if strings.Contains(f.path, mDNSResponderSocket) || strings.Contains(f.path, "SockaddrCtl") {
			t.Errorf("a resolver socket is listed as the tests' own: %s", f)
		}
	}
}
