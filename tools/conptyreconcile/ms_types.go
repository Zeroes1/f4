// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

// coordinate is the small value object used by the pinned buffer and cursor
// code.  It intentionally remains in UTF-16 cell coordinates, matching
// COORD/til::point rather than Go rune or byte offsets.
type coordinate struct {
	x int
	y int
}
