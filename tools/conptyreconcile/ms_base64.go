// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful transcription of src/terminal/parser/base64.cpp from the
// pinned OpenConsole revision. The final UTF-8-to-UTF-16 conversion is the
// documented CP_UTF8, flags-zero boundary already isolated in ms_utf8.go.

package main

import (
	"unicode/utf16"
	"unicode/utf8"
)

const pinnedBase64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func pinnedBase64Space(unit uint16) bool { return unit == '\r' || unit == '\n' }

func pinnedBase64Decode(src []uint16) ([]uint16, bool) {
	length := len(src) / 4 * 3
	if length == 0 {
		return nil, false
	}
	decoded := make([]byte, 0, length)
	state := 0
	var tmp byte
	iter := 0
	for iter < len(src) {
		if pinnedBase64Space(src[iter]) {
			iter++
			continue
		}
		if src[iter] == '=' {
			break
		}
		if src[iter] > 0x7f {
			return nil, false
		}
		pos := -1
		for i := 0; i < len(pinnedBase64Alphabet); i++ {
			if pinnedBase64Alphabet[i] == byte(src[iter]) {
				pos = i
				break
			}
		}
		if pos < 0 {
			return nil, false
		}
		switch state {
		case 0:
			tmp = byte(pos) << 2
			state = 1
		case 1:
			tmp |= byte(pos) >> 4
			decoded = append(decoded, tmp)
			tmp = byte(pos&0x0f) << 4
			state = 2
		case 2:
			tmp |= byte(pos) >> 2
			decoded = append(decoded, tmp)
			tmp = byte(pos&0x03) << 6
			state = 3
		case 3:
			tmp |= byte(pos)
			decoded = append(decoded, tmp)
			state = 0
		}
		iter++
	}

	if iter < len(src) {
		iter++
		switch state {
		case 0, 1:
			return nil, false
		case 2:
			for iter < len(src) && pinnedBase64Space(src[iter]) {
				iter++
			}
			if iter == len(src) || src[iter] != '=' {
				return nil, false
			}
			iter++
			fallthrough
		case 3:
			for iter < len(src) {
				if !pinnedBase64Space(src[iter]) {
					return nil, false
				}
				iter++
			}
		}
	} else if state != 0 {
		return nil, false
	}

	// Base64::s_Decode delegates this boundary to til::u8u16. Invalid UTF-8
	// makes that conversion fail; valid bytes have the same UTF-16 result.
	if !utf8.Valid(decoded) {
		return nil, false
	}
	return utf16.Encode([]rune(string(decoded))), true
}
