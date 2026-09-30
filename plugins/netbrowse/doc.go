// Package netbrowse is f4's built-in network browser for Windows (f4#1702), in
// the spirit of Far Manager's Network panel: the network's providers and
// domains or workgroups, the servers in them and the shares of each server, as
// the system itself enumerates them (WNetOpenEnum and WNetEnumResource from
// mpr.dll), so what shows is what Explorer's "Network" shows and needs no
// protocol client of its own.
//
// Part 1 is the read-only browser: a panel that descends into every container
// (Enter) and back up (the ".." row) and lists the resources at each level.
// Opening a share as an ordinary directory, and a network:// or \\host entry,
// are the next parts. Off Windows the plugin registers nothing.
package netbrowse
