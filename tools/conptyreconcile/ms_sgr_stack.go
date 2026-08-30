// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful transcription of src/types/sgrStack.cpp and
// src/types/inc/sgrStack.hpp from the pinned OpenConsole revision.

package main

// sgrStackState is the ten-entry ring used by SgrStack.  The bit positions
// deliberately use the numeric SgrSaveRestoreStackOptions values from
// DispatchTypes.hpp, including the unused positions between 9 and 21.
type sgrStackState struct {
	stored   [10]sgrSavedAttributes
	nextPush int
	numSaved int
}

type sgrSavedAttributes struct {
	attributes textAttribute
	valid      uint64
}

func (s *sgrStackState) push(current textAttribute, options []int) {
	var valid uint64
	if len(options) == 0 {
		valid |= uint64(1) // SgrSaveRestoreStackOptions::All
	} else {
		for _, option := range options {
			// VTParameter::value_or(0): an omitted option is All.  The
			// source bitset ignores values outside its Max-sized range.
			if option >= 0 && option <= 31 {
				valid |= uint64(1) << uint(option)
			}
		}
	}
	if s.numSaved < len(s.stored) {
		s.numSaved++
	}
	s.stored[s.nextPush] = sgrSavedAttributes{attributes: current, valid: valid}
	s.nextPush = (s.nextPush + 1) % len(s.stored)
}

func (s *sgrStackState) pop(current textAttribute) textAttribute {
	if s.numSaved == 0 {
		return current
	}
	s.numSaved--
	if s.nextPush == 0 {
		s.nextPush = len(s.stored) - 1
	} else {
		s.nextPush--
	}
	saved := s.stored[s.nextPush]
	if saved.valid&(uint64(1)<<0) != 0 {
		return saved.attributes
	}
	result := current
	if saved.valid&(uint64(1)<<1) != 0 {
		result.setBold(saved.attributes.isBold())
	}
	if saved.valid&(uint64(1)<<2) != 0 {
		result.setFaint(saved.attributes.isFaint())
	}
	if saved.valid&(uint64(1)<<3) != 0 {
		result.setItalic(saved.attributes.isItalic())
	}
	if saved.valid&(uint64(1)<<4) != 0 {
		result.setUnderlined(saved.attributes.isUnderlined())
	}
	if saved.valid&(uint64(1)<<5) != 0 {
		result.setBlinking(saved.attributes.isBlinking())
	}
	if saved.valid&(uint64(1)<<7) != 0 {
		result.setReverseVideo(saved.attributes.isReverseVideo())
	}
	if saved.valid&(uint64(1)<<8) != 0 {
		result.setInvisible(saved.attributes.isInvisible())
	}
	if saved.valid&(uint64(1)<<9) != 0 {
		result.setCrossedOut(saved.attributes.isCrossedOut())
	}
	if saved.valid&(uint64(1)<<21) != 0 {
		result.setDoublyUnderlined(saved.attributes.isDoublyUnderlined())
	}
	if saved.valid&(uint64(1)<<30) != 0 {
		result.foreground = saved.attributes.foreground
	}
	if saved.valid&(uint64(1)<<31) != 0 {
		result.background = saved.attributes.background
	}
	return result
}
