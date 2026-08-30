// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful transcription of Utils::ColorFromXParseColorSpec and
// Utils::ColorFromXOrgAppColorName from the pinned OpenConsole source. The
// tables are the literal values from src/types/colorTable.cpp. Values are
// stored as COLORREF (0x00BBGGRR), as they are in the Windows adapter.

package main

import "unicode"

var xorgAppVariantColorTable = map[string][5]uint32{
	"antiquewhite":   {0xD7EBFA, 0xDBEFFF, 0xCCDFEE, 0xB0C0CD, 0x78838B},
	"aquamarine":     {0xD4FF7F, 0xD4FF7F, 0xC6EE76, 0xAACD66, 0x748B45},
	"azure":          {0xFFFFF0, 0xFFFFF0, 0xEEEEE0, 0xCDCDC1, 0x8B8B83},
	"bisque":         {0xC4E4FF, 0xC4E4FF, 0xB7D5EE, 0x9EB7CD, 0x6B7D8B},
	"blue":           {0xFF0000, 0xFF0000, 0xEE0000, 0xCD0000, 0x8B0000},
	"brown":          {0x2A2AA5, 0x4040FF, 0x3B3BEE, 0x3333CD, 0x23238B},
	"burlywood":      {0x87B8DE, 0x9BD3FF, 0x91C5EE, 0x7DAACD, 0x55738B},
	"cadetblue":      {0xA09E5F, 0xFFF598, 0xEEE58E, 0xCDC57A, 0x8B8653},
	"chartreuse":     {0x00FF7F, 0x00FF7F, 0x00EE76, 0x00CD66, 0x008B45},
	"chocolate":      {0x1E69D2, 0x247FFF, 0x2176EE, 0x1D66CD, 0x13458B},
	"coral":          {0x507FFF, 0x5672FF, 0x506AEE, 0x455BCD, 0x2F3E8B},
	"cornsilk":       {0xDCF8FF, 0xDCF8FF, 0xCDE8EE, 0xB1C8CD, 0x78888B},
	"cyan":           {0xFFFF00, 0xFFFF00, 0xEEEE00, 0xCDCD00, 0x8B8B00},
	"darkgoldenrod":  {0x0B86B8, 0x0FB9FF, 0x0EADEE, 0x0C95CD, 0x08658B},
	"darkolivegreen": {0x2F6B55, 0x70FFCA, 0x68EEBC, 0x5ACDA2, 0x3D8B6E},
	"darkorange":     {0x008CFF, 0x007FFF, 0x0076EE, 0x0066CD, 0x00458B},
	"darkorchid":     {0xCC3299, 0xFF3EBF, 0xEE3AB2, 0xCD329A, 0x8B2268},
	"darkseagreen":   {0x8FBC8F, 0xC1FFC1, 0xB4EEB4, 0x9BCD9B, 0x698B69},
	"darkslategray":  {0x4F4F2F, 0xFFFF97, 0xEEEE8D, 0xCDCD79, 0x8B8B52},
	"deeppink":       {0x9314FF, 0x9314FF, 0x8912EE, 0x7610CD, 0x500A8B},
	"deepskyblue":    {0xFFBF00, 0xFFBF00, 0xEEB200, 0xCD9A00, 0x8B6800},
	"dodgerblue":     {0xFF901E, 0xFF901E, 0xEE861C, 0xCD7418, 0x8B4E10},
	"firebrick":      {0x2222B2, 0x3030FF, 0x2C2CEE, 0x2626CD, 0x1A1A8B},
	"gold":           {0x00D7FF, 0x00D7FF, 0x00C9EE, 0x00ADCD, 0x00758B},
	"goldenrod":      {0x20A5DA, 0x25C1FF, 0x22B4EE, 0x1D9BCD, 0x14698B},
	"green":          {0x00FF00, 0x00FF00, 0x00EE00, 0x00CD00, 0x008B00},
	"honeydew":       {0xF0FFF0, 0xF0FFF0, 0xE0EEE0, 0xC1CDC1, 0x838B83},
	"hotpink":        {0xB469FF, 0xB46EFF, 0xA76AEE, 0x9060CD, 0x623A8B},
	"indianred":      {0x5C5CCD, 0x6A6AFF, 0x6363EE, 0x5555CD, 0x3A3A8B},
	"ivory":          {0xF0FFFF, 0xF0FFFF, 0xE0EEEE, 0xC1CDCD, 0x838B8B},
	"khaki":          {0x8CE6F0, 0x8FF6FF, 0x85E6EE, 0x73C6CD, 0x4E868B},
	"lavenderblush":  {0xF5F0FF, 0xF5F0FF, 0xE5E0EE, 0xC5C1CD, 0x86838B},
	"lemonchiffon":   {0xCDFAFF, 0xCDFAFF, 0xBFE9EE, 0xA5C9CD, 0x70898B},
	"lightblue":      {0xE6D8AD, 0xFFEFBF, 0xEEDFB2, 0xCDC09A, 0x8B8368},
	"lightcyan":      {0xFFFFE0, 0xFFFFE0, 0xEEEED1, 0xCDCDB4, 0x8B8B7A},
	"lightgoldenrod": {0x82DDEE, 0x8BECFF, 0x82DCEE, 0x70BECD, 0x4C818B},
	"lightpink":      {0xC1B6FF, 0xB9AEFF, 0xADA2EE, 0x958CCD, 0x655F8B},
	"lightsalmon":    {0x7AA0FF, 0x7AA0FF, 0x7295EE, 0x6281CD, 0x42578B},
	"lightskyblue":   {0xFACE87, 0xFFE2B0, 0xEED3A4, 0xCDB68D, 0x8B7B60},
	"lightsteelblue": {0xDEC4B0, 0xFFE1CA, 0xEED2BC, 0xCDB5A2, 0x8B7B6E},
	"lightyellow":    {0xE0FFFF, 0xE0FFFF, 0xD1EEEE, 0xB4CDCD, 0x7A8B8B},
	"magenta":        {0xFF00FF, 0xFF00FF, 0xEE00EE, 0xCD00CD, 0x8B008B},
	"maroon":         {0x6030B0, 0xB334FF, 0xA730EE, 0x9029CD, 0x621C8B},
	"mediumorchid":   {0xD355BA, 0xFF66E0, 0xEE5FD1, 0xCD52B4, 0x8B377A},
	"mediumpurple":   {0xDB7093, 0xFF82AB, 0xEE799F, 0xCD6889, 0x8B475D},
	"mistyrose":      {0xE1E4FF, 0xE1E4FF, 0xD2D5EE, 0xB5B7CD, 0x7B7D8B},
	"navajowhite":    {0xADDEFF, 0xADDEFF, 0xA1CFEE, 0x8BB3CD, 0x5E798B},
	"olivedrab":      {0x238E6B, 0x3EFFC0, 0x3AEEB3, 0x32CD9A, 0x228B69},
	"orange":         {0x00A5FF, 0x00A5FF, 0x009AEE, 0x0085CD, 0x005A8B},
	"orangered":      {0x0045FF, 0x0045FF, 0x0040EE, 0x0037CD, 0x00258B},
	"orchid":         {0xD670DA, 0xFA83FF, 0xE97AEE, 0xC969CD, 0x89478B},
	"palegreen":      {0x98FB98, 0x9AFF9A, 0x90EE90, 0x7CCD7C, 0x548B54},
	"paleturquoise":  {0xEEEEAF, 0xFFFFBB, 0xEEEEAE, 0xCDCD96, 0x8B8B66},
	"palevioletred":  {0x9370DB, 0xAB82FF, 0x9F79EE, 0x8968CD, 0x5D478B},
	"peachpuff":      {0xB9DAFF, 0xB9DAFF, 0xADCBEE, 0x95AFCD, 0x65778B},
	"pink":           {0xCBC0FF, 0xC5B5FF, 0xB8A9EE, 0x9E91CD, 0x6C638B},
	"plum":           {0xDDA0DD, 0xFFBBFF, 0xEEAEEE, 0xCD96CD, 0x8B668B},
	"purple":         {0xF020A0, 0xFF309B, 0xEE2C91, 0xCD267D, 0x8B1A55},
	"red":            {0x0000FF, 0x0000FF, 0x0000EE, 0x0000CD, 0x00008B},
	"rosybrown":      {0x8F8FBC, 0xC1C1FF, 0xB4B4EE, 0x9B9BCD, 0x69698B},
	"royalblue":      {0xE16941, 0xFF7648, 0xEE6E43, 0xCD5F3A, 0x8B4027},
	"salmon":         {0x7280FA, 0x698CFF, 0x6282EE, 0x5470CD, 0x394C8B},
	"seagreen":       {0x578B2E, 0x9FFF54, 0x94EE4E, 0x80CD43, 0x578B2E},
	"seashell":       {0xEEF5FF, 0xEEF5FF, 0xDEE5EE, 0xBFC5CD, 0x82868B},
	"sienna":         {0x2D52A0, 0x4782FF, 0x4279EE, 0x3968CD, 0x26478B},
	"skyblue":        {0xEBCE87, 0xFFCE87, 0xEEC07E, 0xCDA66C, 0x8B704A},
	"slateblue":      {0xCD5A6A, 0xFF6F83, 0xEE677A, 0xCD5969, 0x8B3C47},
	"slategray":      {0x908070, 0xFFE2C6, 0xEED3B9, 0xCDB69F, 0x8B7B6C},
	"snow":           {0xFAFAFF, 0xFAFAFF, 0xE9E9EE, 0xC9C9CD, 0x89898B},
	"springgreen":    {0x7FFF00, 0x7FFF00, 0x76EE00, 0x66CD00, 0x458B00},
	"steelblue":      {0xB48246, 0xFFB863, 0xEEAC5C, 0xCD944F, 0x8B6436},
	"tan":            {0x8CB4D2, 0x4FA5FF, 0x499AEE, 0x3F85CD, 0x2B5A8B},
	"thistle":        {0xD8BFD8, 0xFFE1FF, 0xEED2EE, 0xCDB5CD, 0x8B7B8B},
	"tomato":         {0x4763FF, 0x4763FF, 0x425CEE, 0x394FCD, 0x26368B},
	"turquoise":      {0xD0E040, 0xFFF500, 0xEEE500, 0xCDC500, 0x8B8600},
	"violetred":      {0x9020D0, 0x963EFF, 0x8C3AEE, 0x7832CD, 0x52228B},
	"wheat":          {0xB3DEF5, 0xBAE7FF, 0xAED8EE, 0x96BACD, 0x667E8B},
	"yellow":         {0x00FFFF, 0x00FFFF, 0x00EEEE, 0x00CDCD, 0x008B8B},
}

var xorgAppColorTable = map[string]uint32{
	"aliceblue": 0xFFF8F0, "aqua": 0xFFFF00, "beige": 0xDCF5F5, "black": 0x000000,
	"blanchedalmond": 0xCDEBFF, "blueviolet": 0xE22B8A, "cornflowerblue": 0xED9564,
	"crimson": 0x3C14DC, "darkblue": 0x8B0000, "darkcyan": 0x8B8B00, "darkgray": 0xA9A9A9,
	"darkgreen": 0x006400, "darkgrey": 0xA9A9A9, "darkkhaki": 0x6BB7BD,
	"darkmagenta": 0x8B008B, "darkred": 0x00008B, "darksalmon": 0x7A96E9,
	"darkslateblue": 0x8B3D48, "darkslategrey": 0x4F4F2F, "darkturquoise": 0xD1CE00,
	"darkviolet": 0xD30094, "dimgray": 0x696969, "dimgrey": 0x696969,
	"floralwhite": 0xF0FAFF, "forestgreen": 0x228B22, "fuchsia": 0xFF00FF,
	"gainsboro": 0xDCDCDC, "ghostwhite": 0xFFF8F8, "gray": 0xBEBEBE,
	"greenyellow": 0x2FFFAD, "grey": 0xBEBEBE, "indigo": 0x82004B, "lavender": 0xFAE6E6,
	"lawngreen": 0x00FC7C, "lightcoral": 0x8080F0, "lightgoldenrodyellow": 0xD2FAFA,
	"lightgray": 0xD3D3D3, "lightgreen": 0x90EE90, "lightgrey": 0xD3D3D3,
	"lightseagreen": 0xAAB220, "lightslateblue": 0xFF7084, "lightslategray": 0x998877,
	"lightslategrey": 0x998877, "lime": 0x00FF00, "limegreen": 0x32CD32, "linen": 0xE6F0FA,
	"mediumaquamarine": 0xAACD66, "mediumblue": 0xCD0000, "mediumseagreen": 0x71B33C,
	"mediumslateblue": 0xEE687B, "mediumspringgreen": 0x9AFA00, "mediumturquoise": 0xCCD148,
	"mediumvioletred": 0x8515C7, "midnightblue": 0x701919, "mintcream": 0xFAFFF5,
	"moccasin": 0xB5E4FF, "navy": 0x800000, "navyblue": 0x800000, "oldlace": 0xE6F5FD,
	"olive": 0x008080, "palegoldenrod": 0xAAE8EE, "papayawhip": 0xD5EFFF, "peru": 0x3F85CD,
	"powderblue": 0xE6E0B0, "rebeccapurple": 0x993366, "saddlebrown": 0x13458B,
	"sandybrown": 0x60A4F4, "silver": 0xC0C0C0, "slategrey": 0x908070, "teal": 0x808000,
	"violet": 0xEE82EE, "webgray": 0x808080, "webgreen": 0x008000, "webgrey": 0x808080,
	"webmaroon": 0x000080, "webpurple": 0x800080, "white": 0xFFFFFF, "whitesmoke": 0xF5F5F5,
	"x11gray": 0xBEBEBE, "x11green": 0x00FF00, "x11grey": 0xBEBEBE, "x11maroon": 0x6030B0,
	"x11purple": 0xF020A0, "yellowgreen": 0x32CD9A,
}

func colorFromXTermColor(value string) (uint32, bool) {
	if color, ok := colorFromXParseColorSpec(value); ok {
		return color, true
	}
	return colorFromXOrgAppColorName(value)
}

func hexToUint(ch rune) (uint32, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return uint32(ch - '0'), true
	case ch >= 'A' && ch <= 'F':
		return uint32(ch-'A') + 10, true
	case ch >= 'a' && ch <= 'f':
		return uint32(ch-'a') + 10, true
	default:
		return 0, false
	}
}

func colorFromXParseColorSpec(value string) (uint32, bool) {
	chars := []rune(value)
	stringSize := len(chars)
	foundXParseColorSpec := false
	foundValidColorSpec := false
	isSharpSignFormat := false
	rgbHexDigitCount := 0
	parameterValues := [3]uint32{}
	colorValues := [3]uint32{}
	curr := 0
	if stringSize > 4 {
		prefix := make([]rune, 4)
		copy(prefix, chars[:4])
		for i, ch := range prefix {
			if ch >= 'A' && ch <= 'Z' {
				prefix[i] = ch + ('a' - 'A')
			}
		}
		if string(prefix) == "rgb:" {
			if stringSize < 9 || stringSize > 18 {
				return 0, false
			}
			foundXParseColorSpec = true
			curr = 4
		}
	}
	if !foundXParseColorSpec && stringSize > 1 && chars[0] == '#' {
		if stringSize != 4 && stringSize != 7 && stringSize != 10 && stringSize != 13 {
			return 0, false
		}
		isSharpSignFormat = true
		foundXParseColorSpec = true
		rgbHexDigitCount = (stringSize - 1) / 3
		curr = 1
	}
	if !foundXParseColorSpec {
		return 0, false
	}
	for component := 0; component < 3; component++ {
		foundColor := false
		iteration := 4
		if isSharpSignFormat {
			iteration = rgbHexDigitCount
		}
		for i := 0; i < iteration && curr < len(chars); i++ {
			intVal, ok := hexToUint(chars[curr])
			if !ok {
				return 0, false
			}
			curr++
			parameterValues[component] *= 16
			parameterValues[component] += intVal
			if isSharpSignFormat {
				foundColor = true
				if i >= rgbHexDigitCount {
					break
				}
			} else {
				rgbHexDigitCount = i + 1
				if component < 2 && curr < len(chars) && chars[curr] == '/' {
					curr++
					foundColor = true
					break
				} else if curr >= len(chars) {
					foundColor = true
					break
				}
			}
		}
		if !foundColor {
			return 0, false
		}
		scaleMultiplier := uint32(0x11)
		if isSharpSignFormat {
			scaleMultiplier = 0x10
		}
		scaleDivisor := (scaleMultiplier << 8) >> uint(4*(4-rgbHexDigitCount))
		colorValues[component] = parameterValues[component] * scaleMultiplier / scaleDivisor
	}
	if curr >= len(chars) {
		foundValidColorSpec = true
	}
	if !foundValidColorSpec {
		return 0, false
	}
	return colorValues[0]&0xff | (colorValues[1]&0xff)<<8 | (colorValues[2]&0xff)<<16, true
}

func colorFromXOrgAppColorName(value string) (uint32, bool) {
	variantIndex := uint64(0)
	foundVariant := false
	name := make([]rune, 0, len(value))
	for _, ch := range value {
		if ch > 127 {
			return 0, false
		}
		if ch >= '0' && ch <= '9' {
			foundVariant = true
			variantIndex *= 10
			variantIndex += uint64(ch - '0')
			continue
		}
		if unicode.IsSpace(ch) {
			continue
		}
		if foundVariant {
			return 0, false
		}
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		name = append(name, ch)
	}
	key := string(name)
	if colors, ok := xorgAppVariantColorTable[key]; ok && variantIndex < uint64(len(colors)) {
		return colors[variantIndex], true
	}
	if (key == "gray" || key == "grey") && foundVariant {
		if variantIndex > 100 {
			return 0, false
		}
		component := uint32((variantIndex*255 + 50) / 100)
		return component | component<<8 | component<<16, true
	}
	color, ok := xorgAppColorTable[key]
	return color, ok
}

// splitPinnedString is Utils::SplitString. It deliberately retains an empty
// final component when the delimiter terminates the input.
func splitPinnedString(value []uint16, delimiter uint16) [][]uint16 {
	result := make([][]uint16, 0)
	current := 0
	for current < len(value) {
		next := current
		for next < len(value) && value[next] != delimiter {
			next++
		}
		result = append(result, append([]uint16(nil), value[current:next]...))
		if next == len(value) {
			break
		}
		current = next + 1
		if current >= len(value) {
			result = append(result, []uint16{})
		}
	}
	return result
}

// stringToPinnedUint is Utils::StringToUint. The source accepts only one or
// more ASCII decimal digits and performs unsigned accumulation.
func stringToPinnedUint(value []uint16) (uint32, bool) {
	if len(value) == 0 {
		return 0, false
	}
	var result uint32
	for _, unit := range value {
		if unit < '0' || unit > '9' {
			return 0, false
		}
		result *= 10
		result += uint32(unit - '0')
	}
	return result, true
}
