// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful representation of TextColor and TextAttribute from the
// pinned OpenConsole tree.  The representation is Go-native, but the fields
// and mutators preserve the source's default/indexed/RGB and extended
// attribute state instead of collapsing it to a legacy WORD.

package main

type textColorKind uint8

const (
	textColorDefault textColorKind = iota
	textColorIndex16
	textColorIndex256
	textColorRGB
)

type textColor struct {
	kind  textColorKind
	index uint8
	rgb   uint32
}

type textAttribute struct {
	legacy      uint16
	foreground  textColor
	background  textColor
	extended    uint8
	hyperlinkID uint16
}

const (
	extBold             uint8 = 0x01
	extItalics          uint8 = 0x02
	extBlinking         uint8 = 0x04
	extInvisible        uint8 = 0x08
	extCrossedOut       uint8 = 0x10
	extUnderlined       uint8 = 0x20
	extDoublyUnderlined uint8 = 0x40
	extFaint            uint8 = 0x80
)

func (a *textAttribute) setFlag(mask uint8, enabled bool) {
	if enabled {
		a.extended |= mask
	} else {
		a.extended &^= mask
	}
}

func (a textAttribute) hasFlag(mask uint8) bool { return a.extended&mask != 0 }

func (a *textAttribute) setDefaultForeground() { a.foreground = textColor{} }
func (a *textAttribute) setDefaultBackground() { a.background = textColor{} }

func (a *textAttribute) setDefaultMetaAttrs() {
	a.extended = 0
	a.legacy = 0
}

func (a *textAttribute) setStandardErase() {
	a.setDefaultMetaAttrs()
	a.hyperlinkID = 0
}

func xtermToWindowsIndex(index int) uint8 {
	var result uint8
	if index&0x01 != 0 {
		result |= 0x04 // FOREGROUND_RED
	}
	if index&0x02 != 0 {
		result |= 0x02 // FOREGROUND_GREEN
	}
	if index&0x04 != 0 {
		result |= 0x01 // FOREGROUND_BLUE
	}
	if index&0x08 != 0 {
		result |= 0x08 // FOREGROUND_INTENSITY
	}
	return result
}

func xterm256ToWindowsIndex(index int) uint8 {
	if index < 16 {
		return xtermToWindowsIndex(index)
	}
	return uint8(index)
}

func (a *textAttribute) setIndexedForeground(index uint8) {
	a.foreground = textColor{kind: textColorIndex16, index: xtermToWindowsIndex(int(index))}
}

func (a *textAttribute) setIndexedBackground(index uint8) {
	// Background legacy indices use the same xterm-to-Windows bit mapping,
	// shifted in the serialized legacy word only; the TextColor itself stores
	// the unshifted Windows index in the pinned implementation.
	a.background = textColor{kind: textColorIndex16, index: xtermToWindowsIndex(int(index))}
}

func (a *textAttribute) setIndexedForeground256(index uint8) {
	a.foreground = textColor{kind: textColorIndex256, index: xterm256ToWindowsIndex(int(index))}
}

func (a *textAttribute) setIndexedBackground256(index uint8) {
	a.background = textColor{kind: textColorIndex256, index: xterm256ToWindowsIndex(int(index))}
}

func (a *textAttribute) setColor(rgb uint32, foreground bool) {
	color := textColor{kind: textColorRGB, rgb: rgb}
	if foreground {
		a.foreground = color
	} else {
		a.background = color
	}
}

func (a *textAttribute) setHyperlinkID(id uint16) { a.hyperlinkID = id }

func (a textAttribute) isBold() bool             { return a.hasFlag(extBold) }
func (a textAttribute) isFaint() bool            { return a.hasFlag(extFaint) }
func (a textAttribute) isItalic() bool           { return a.hasFlag(extItalics) }
func (a textAttribute) isBlinking() bool         { return a.hasFlag(extBlinking) }
func (a textAttribute) isInvisible() bool        { return a.hasFlag(extInvisible) }
func (a textAttribute) isCrossedOut() bool       { return a.hasFlag(extCrossedOut) }
func (a textAttribute) isUnderlined() bool       { return a.hasFlag(extUnderlined) }
func (a textAttribute) isDoublyUnderlined() bool { return a.hasFlag(extDoublyUnderlined) }
func (a textAttribute) isOverlined() bool        { return a.legacy&0x0001 != 0 }
func (a textAttribute) isReverseVideo() bool     { return a.legacy&0x4000 != 0 }

func (a *textAttribute) setBold(v bool)             { a.setFlag(extBold, v) }
func (a *textAttribute) setFaint(v bool)            { a.setFlag(extFaint, v) }
func (a *textAttribute) setItalic(v bool)           { a.setFlag(extItalics, v) }
func (a *textAttribute) setBlinking(v bool)         { a.setFlag(extBlinking, v) }
func (a *textAttribute) setInvisible(v bool)        { a.setFlag(extInvisible, v) }
func (a *textAttribute) setCrossedOut(v bool)       { a.setFlag(extCrossedOut, v) }
func (a *textAttribute) setUnderlined(v bool)       { a.setFlag(extUnderlined, v) }
func (a *textAttribute) setDoublyUnderlined(v bool) { a.setFlag(extDoublyUnderlined, v) }
func (a *textAttribute) setOverlined(v bool) {
	if v {
		a.legacy |= 0x0001
	} else {
		a.legacy &^= 0x0001
	}
}
func (a *textAttribute) setReverseVideo(v bool) {
	if v {
		a.legacy |= 0x4000
	} else {
		a.legacy &^= 0x4000
	}
}

func (a textAttribute) isHyperlink() bool { return a.hyperlinkID != 0 }
