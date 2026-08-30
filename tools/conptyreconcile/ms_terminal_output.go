// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful transcription of the pinned terminalOutput.cpp and
// charsets.hpp (e9b4e2e18fb1b9cee6839969d42cd0f95d228926). The table
// entries below are mechanically derived from that pinned charsets.hpp.
package main

type charsetReplacement struct {
	from uint16
	to   uint16
}

type charsetTable struct {
	base         uint16
	size         uint16
	replacements map[uint16]uint16
	name         string
}

func newCharsetTable(name string, base, size uint16, pairs []charsetReplacement) *charsetTable {
	table := &charsetTable{base: base, size: size, name: name, replacements: make(map[uint16]uint16, len(pairs))}
	for _, pair := range pairs {
		table.replacements[pair.from] = pair.to
	}
	return table
}

func (t *charsetTable) translate(index uint16) uint16 {
	if t == nil {
		return index
	}
	if uint32(index) < uint32(t.size) {
		codepoint := t.base + index
		if replacement, ok := t.replacements[codepoint]; ok {
			return replacement
		}
		return codepoint
	}
	return index
}

var (
	msASCIICharset  = newCharsetTable("Ascii", 0x20, 95, nil)
	msLatin1Charset = newCharsetTable("Latin1", 0xa0, 96, nil)
	msLatin2Charset = newCharsetTable("Latin2", 0xa0, 96, []charsetReplacement{
		{from: 0x00a1, to: 0x0104},
		{from: 0x00a2, to: 0x02d8},
		{from: 0x00a3, to: 0x0141},
		{from: 0x00a5, to: 0x013d},
		{from: 0x00a6, to: 0x015a},
		{from: 0x00a9, to: 0x0160},
		{from: 0x00aa, to: 0x015e},
		{from: 0x00ab, to: 0x0164},
		{from: 0x00ac, to: 0x0179},
		{from: 0x00ae, to: 0x017d},
		{from: 0x00af, to: 0x017b},
		{from: 0x00b1, to: 0x0105},
		{from: 0x00b2, to: 0x02db},
		{from: 0x00b3, to: 0x0142},
		{from: 0x00b5, to: 0x013e},
		{from: 0x00b6, to: 0x015b},
		{from: 0x00b7, to: 0x02c7},
		{from: 0x00b9, to: 0x0161},
		{from: 0x00ba, to: 0x015f},
		{from: 0x00bb, to: 0x0165},
		{from: 0x00bc, to: 0x017a},
		{from: 0x00bd, to: 0x02dd},
		{from: 0x00be, to: 0x017e},
		{from: 0x00bf, to: 0x017c},
		{from: 0x00c0, to: 0x0154},
		{from: 0x00c3, to: 0x0102},
		{from: 0x00c5, to: 0x0139},
		{from: 0x00c6, to: 0x0106},
		{from: 0x00c8, to: 0x010c},
		{from: 0x00ca, to: 0x0118},
		{from: 0x00cc, to: 0x011a},
		{from: 0x00cf, to: 0x010e},
		{from: 0x00d0, to: 0x0110},
		{from: 0x00d1, to: 0x0143},
		{from: 0x00d2, to: 0x0147},
		{from: 0x00d5, to: 0x0150},
		{from: 0x00d8, to: 0x0158},
		{from: 0x00d9, to: 0x016e},
		{from: 0x00db, to: 0x0170},
		{from: 0x00de, to: 0x0162},
		{from: 0x00e0, to: 0x0155},
		{from: 0x00e3, to: 0x0103},
		{from: 0x00e5, to: 0x013a},
		{from: 0x00e6, to: 0x0107},
		{from: 0x00e8, to: 0x010d},
		{from: 0x00ea, to: 0x0119},
		{from: 0x00ec, to: 0x011b},
		{from: 0x00ef, to: 0x010f},
		{from: 0x00f0, to: 0x0111},
		{from: 0x00f1, to: 0x0144},
		{from: 0x00f2, to: 0x0148},
		{from: 0x00f5, to: 0x0151},
		{from: 0x00f8, to: 0x0159},
		{from: 0x00f9, to: 0x016f},
		{from: 0x00fb, to: 0x0171},
		{from: 0x00fe, to: 0x0163},
		{from: 0x00ff, to: 0x02d9},
	})
	msLatinCyrillicCharset = newCharsetTable("LatinCyrillic", 0xa0, 96, []charsetReplacement{
		{from: 0x00a1, to: 0x0401},
		{from: 0x00a2, to: 0x0402},
		{from: 0x00a3, to: 0x0403},
		{from: 0x00a4, to: 0x0404},
		{from: 0x00a5, to: 0x0405},
		{from: 0x00a6, to: 0x0406},
		{from: 0x00a7, to: 0x0407},
		{from: 0x00a8, to: 0x0408},
		{from: 0x00a9, to: 0x0409},
		{from: 0x00aa, to: 0x040a},
		{from: 0x00ab, to: 0x040b},
		{from: 0x00ac, to: 0x040c},
		{from: 0x00ae, to: 0x040e},
		{from: 0x00af, to: 0x040f},
		{from: 0x00b0, to: 0x0410},
		{from: 0x00b1, to: 0x0411},
		{from: 0x00b2, to: 0x0412},
		{from: 0x00b3, to: 0x0413},
		{from: 0x00b4, to: 0x0414},
		{from: 0x00b5, to: 0x0415},
		{from: 0x00b6, to: 0x0416},
		{from: 0x00b7, to: 0x0417},
		{from: 0x00b8, to: 0x0418},
		{from: 0x00b9, to: 0x0419},
		{from: 0x00ba, to: 0x041a},
		{from: 0x00bb, to: 0x041b},
		{from: 0x00bc, to: 0x041c},
		{from: 0x00bd, to: 0x041d},
		{from: 0x00be, to: 0x041e},
		{from: 0x00bf, to: 0x041f},
		{from: 0x00c0, to: 0x0420},
		{from: 0x00c1, to: 0x0421},
		{from: 0x00c2, to: 0x0422},
		{from: 0x00c3, to: 0x0423},
		{from: 0x00c4, to: 0x0424},
		{from: 0x00c5, to: 0x0425},
		{from: 0x00c6, to: 0x0426},
		{from: 0x00c7, to: 0x0427},
		{from: 0x00c8, to: 0x0428},
		{from: 0x00c9, to: 0x0429},
		{from: 0x00ca, to: 0x042a},
		{from: 0x00cb, to: 0x042b},
		{from: 0x00cc, to: 0x042c},
		{from: 0x00cd, to: 0x042d},
		{from: 0x00ce, to: 0x042e},
		{from: 0x00cf, to: 0x042f},
		{from: 0x00d0, to: 0x0430},
		{from: 0x00d1, to: 0x0431},
		{from: 0x00d2, to: 0x0432},
		{from: 0x00d3, to: 0x0433},
		{from: 0x00d4, to: 0x0434},
		{from: 0x00d5, to: 0x0435},
		{from: 0x00d6, to: 0x0436},
		{from: 0x00d7, to: 0x0437},
		{from: 0x00d8, to: 0x0438},
		{from: 0x00d9, to: 0x0439},
		{from: 0x00da, to: 0x043a},
		{from: 0x00db, to: 0x043b},
		{from: 0x00dc, to: 0x043c},
		{from: 0x00dd, to: 0x043d},
		{from: 0x00de, to: 0x043e},
		{from: 0x00df, to: 0x043f},
		{from: 0x00e0, to: 0x0440},
		{from: 0x00e1, to: 0x0441},
		{from: 0x00e2, to: 0x0442},
		{from: 0x00e3, to: 0x0443},
		{from: 0x00e4, to: 0x0444},
		{from: 0x00e5, to: 0x0445},
		{from: 0x00e6, to: 0x0446},
		{from: 0x00e7, to: 0x0447},
		{from: 0x00e8, to: 0x0448},
		{from: 0x00e9, to: 0x0449},
		{from: 0x00ea, to: 0x044a},
		{from: 0x00eb, to: 0x044b},
		{from: 0x00ec, to: 0x044c},
		{from: 0x00ed, to: 0x044d},
		{from: 0x00ee, to: 0x044e},
		{from: 0x00ef, to: 0x044f},
		{from: 0x00f0, to: 0x2116},
		{from: 0x00f1, to: 0x0451},
		{from: 0x00f2, to: 0x0452},
		{from: 0x00f3, to: 0x0453},
		{from: 0x00f4, to: 0x0454},
		{from: 0x00f5, to: 0x0455},
		{from: 0x00f6, to: 0x0456},
		{from: 0x00f7, to: 0x0457},
		{from: 0x00f8, to: 0x0458},
		{from: 0x00f9, to: 0x0459},
		{from: 0x00fa, to: 0x045a},
		{from: 0x00fb, to: 0x045b},
		{from: 0x00fc, to: 0x045c},
		{from: 0x00fd, to: 0x00a7},
		{from: 0x00fe, to: 0x045e},
		{from: 0x00ff, to: 0x045f},
	})
	msLatinGreekCharset = newCharsetTable("LatinGreek", 0xa0, 96, []charsetReplacement{
		{from: 0x00a1, to: 0x2018},
		{from: 0x00a2, to: 0x2019},
		{from: 0x00a4, to: 0x2426},
		{from: 0x00a5, to: 0x2426},
		{from: 0x00aa, to: 0x2426},
		{from: 0x00ae, to: 0x2426},
		{from: 0x00af, to: 0x2015},
		{from: 0x00b4, to: 0x0384},
		{from: 0x00b5, to: 0x0385},
		{from: 0x00b6, to: 0x0386},
		{from: 0x00b8, to: 0x0388},
		{from: 0x00b9, to: 0x0389},
		{from: 0x00ba, to: 0x038a},
		{from: 0x00bc, to: 0x038c},
		{from: 0x00be, to: 0x038e},
		{from: 0x00bf, to: 0x038f},
		{from: 0x00c0, to: 0x0390},
		{from: 0x00c1, to: 0x0391},
		{from: 0x00c2, to: 0x0392},
		{from: 0x00c3, to: 0x0393},
		{from: 0x00c4, to: 0x0394},
		{from: 0x00c5, to: 0x0395},
		{from: 0x00c6, to: 0x0396},
		{from: 0x00c7, to: 0x0397},
		{from: 0x00c8, to: 0x0398},
		{from: 0x00c9, to: 0x0399},
		{from: 0x00ca, to: 0x039a},
		{from: 0x00cb, to: 0x039b},
		{from: 0x00cc, to: 0x039c},
		{from: 0x00cd, to: 0x039d},
		{from: 0x00ce, to: 0x039e},
		{from: 0x00cf, to: 0x039f},
		{from: 0x00d0, to: 0x03a0},
		{from: 0x00d1, to: 0x03a1},
		{from: 0x00d2, to: 0x2426},
		{from: 0x00d3, to: 0x03a3},
		{from: 0x00d4, to: 0x03a4},
		{from: 0x00d5, to: 0x03a5},
		{from: 0x00d6, to: 0x03a6},
		{from: 0x00d7, to: 0x03a7},
		{from: 0x00d8, to: 0x03a8},
		{from: 0x00d9, to: 0x03a9},
		{from: 0x00da, to: 0x03aa},
		{from: 0x00db, to: 0x03ab},
		{from: 0x00dc, to: 0x03ac},
		{from: 0x00dd, to: 0x03ad},
		{from: 0x00de, to: 0x03ae},
		{from: 0x00df, to: 0x03af},
		{from: 0x00e0, to: 0x03b0},
		{from: 0x00e1, to: 0x03b1},
		{from: 0x00e2, to: 0x03b2},
		{from: 0x00e3, to: 0x03b3},
		{from: 0x00e4, to: 0x03b4},
		{from: 0x00e5, to: 0x03b5},
		{from: 0x00e6, to: 0x03b6},
		{from: 0x00e7, to: 0x03b7},
		{from: 0x00e8, to: 0x03b8},
		{from: 0x00e9, to: 0x03b9},
		{from: 0x00ea, to: 0x03ba},
		{from: 0x00eb, to: 0x03bb},
		{from: 0x00ec, to: 0x03bc},
		{from: 0x00ed, to: 0x03bd},
		{from: 0x00ee, to: 0x03be},
		{from: 0x00ef, to: 0x03bf},
		{from: 0x00f0, to: 0x03c0},
		{from: 0x00f1, to: 0x03c1},
		{from: 0x00f2, to: 0x03c2},
		{from: 0x00f3, to: 0x03c3},
		{from: 0x00f4, to: 0x03c4},
		{from: 0x00f5, to: 0x03c5},
		{from: 0x00f6, to: 0x03c6},
		{from: 0x00f7, to: 0x03c7},
		{from: 0x00f8, to: 0x03c8},
		{from: 0x00f9, to: 0x03c9},
		{from: 0x00fa, to: 0x03ca},
		{from: 0x00fb, to: 0x03cb},
		{from: 0x00fc, to: 0x03cc},
		{from: 0x00fd, to: 0x03cd},
		{from: 0x00fe, to: 0x03ce},
		{from: 0x00ff, to: 0x2426},
	})
	msLatinHebrewCharset = newCharsetTable("LatinHebrew", 0xa0, 96, []charsetReplacement{
		{from: 0x00a1, to: 0x2426},
		{from: 0x00aa, to: 0x00d7},
		{from: 0x00ba, to: 0x00f7},
		{from: 0x00bf, to: 0x2426},
		{from: 0x00c0, to: 0x2426},
		{from: 0x00c1, to: 0x2426},
		{from: 0x00c2, to: 0x2426},
		{from: 0x00c3, to: 0x2426},
		{from: 0x00c4, to: 0x2426},
		{from: 0x00c5, to: 0x2426},
		{from: 0x00c6, to: 0x2426},
		{from: 0x00c7, to: 0x2426},
		{from: 0x00c8, to: 0x2426},
		{from: 0x00c9, to: 0x2426},
		{from: 0x00ca, to: 0x2426},
		{from: 0x00cb, to: 0x2426},
		{from: 0x00cc, to: 0x2426},
		{from: 0x00cd, to: 0x2426},
		{from: 0x00ce, to: 0x2426},
		{from: 0x00cf, to: 0x2426},
		{from: 0x00d0, to: 0x2426},
		{from: 0x00d1, to: 0x2426},
		{from: 0x00d2, to: 0x2426},
		{from: 0x00d3, to: 0x2426},
		{from: 0x00d4, to: 0x2426},
		{from: 0x00d5, to: 0x2426},
		{from: 0x00d6, to: 0x2426},
		{from: 0x00d7, to: 0x2426},
		{from: 0x00d8, to: 0x2426},
		{from: 0x00d9, to: 0x2426},
		{from: 0x00da, to: 0x2426},
		{from: 0x00db, to: 0x2426},
		{from: 0x00dc, to: 0x2426},
		{from: 0x00dd, to: 0x2426},
		{from: 0x00de, to: 0x2426},
		{from: 0x00df, to: 0x2017},
		{from: 0x00e0, to: 0x05d0},
		{from: 0x00e1, to: 0x05d1},
		{from: 0x00e2, to: 0x05d2},
		{from: 0x00e3, to: 0x05d3},
		{from: 0x00e4, to: 0x05d4},
		{from: 0x00e5, to: 0x05d5},
		{from: 0x00e6, to: 0x05d6},
		{from: 0x00e7, to: 0x05d7},
		{from: 0x00e8, to: 0x05d8},
		{from: 0x00e9, to: 0x05d9},
		{from: 0x00ea, to: 0x05da},
		{from: 0x00eb, to: 0x05db},
		{from: 0x00ec, to: 0x05dc},
		{from: 0x00ed, to: 0x05dd},
		{from: 0x00ee, to: 0x05de},
		{from: 0x00ef, to: 0x05df},
		{from: 0x00f0, to: 0x05e0},
		{from: 0x00f1, to: 0x05e1},
		{from: 0x00f2, to: 0x05e2},
		{from: 0x00f3, to: 0x05e3},
		{from: 0x00f4, to: 0x05e4},
		{from: 0x00f5, to: 0x05e5},
		{from: 0x00f6, to: 0x05e6},
		{from: 0x00f7, to: 0x05e7},
		{from: 0x00f8, to: 0x05e8},
		{from: 0x00f9, to: 0x05e9},
		{from: 0x00fa, to: 0x05ea},
		{from: 0x00fb, to: 0x2426},
		{from: 0x00fc, to: 0x2426},
		{from: 0x00fd, to: 0x200e},
		{from: 0x00fe, to: 0x200f},
		{from: 0x00ff, to: 0x2426},
	})
	msLatin5Charset = newCharsetTable("Latin5", 0xa0, 96, []charsetReplacement{
		{from: 0x00d0, to: 0x011e},
		{from: 0x00dd, to: 0x0130},
		{from: 0x00de, to: 0x015e},
		{from: 0x00f0, to: 0x011f},
		{from: 0x00fd, to: 0x0131},
		{from: 0x00fe, to: 0x015f},
	})
	msDecSupplementalCharset = newCharsetTable("DecSupplemental", 0xa0, 95, []charsetReplacement{
		{from: 0x00a4, to: 0x2426},
		{from: 0x00a6, to: 0x2426},
		{from: 0x00a8, to: 0x00a4},
		{from: 0x00ac, to: 0x2426},
		{from: 0x00ad, to: 0x2426},
		{from: 0x00ae, to: 0x2426},
		{from: 0x00af, to: 0x2426},
		{from: 0x00b4, to: 0x2426},
		{from: 0x00b8, to: 0x2426},
		{from: 0x00be, to: 0x2426},
		{from: 0x00d0, to: 0x2426},
		{from: 0x00d7, to: 0x0152},
		{from: 0x00dd, to: 0x0178},
		{from: 0x00de, to: 0x2426},
		{from: 0x00f0, to: 0x2426},
		{from: 0x00f7, to: 0x0153},
		{from: 0x00fd, to: 0x00ff},
		{from: 0x00fe, to: 0x2426},
	})
	msDecSpecialGraphicsCharset = newCharsetTable("DecSpecialGraphics", 0x20, 95, []charsetReplacement{
		{from: 0x005f, to: 0x0020},
		{from: 0x0060, to: 0x2666},
		{from: 0x0061, to: 0x2592},
		{from: 0x0062, to: 0x2409},
		{from: 0x0063, to: 0x240c},
		{from: 0x0064, to: 0x240d},
		{from: 0x0065, to: 0x240a},
		{from: 0x0066, to: 0x00b0},
		{from: 0x0067, to: 0x00b1},
		{from: 0x0068, to: 0x2424},
		{from: 0x0069, to: 0x240b},
		{from: 0x006a, to: 0x2518},
		{from: 0x006b, to: 0x2510},
		{from: 0x006c, to: 0x250c},
		{from: 0x006d, to: 0x2514},
		{from: 0x006e, to: 0x253c},
		{from: 0x006f, to: 0x23ba},
		{from: 0x0070, to: 0x23bb},
		{from: 0x0071, to: 0x2500},
		{from: 0x0072, to: 0x23bc},
		{from: 0x0073, to: 0x23bd},
		{from: 0x0074, to: 0x251c},
		{from: 0x0075, to: 0x2524},
		{from: 0x0076, to: 0x2534},
		{from: 0x0077, to: 0x252c},
		{from: 0x0078, to: 0x2502},
		{from: 0x0079, to: 0x2264},
		{from: 0x007a, to: 0x2265},
		{from: 0x007b, to: 0x03c0},
		{from: 0x007c, to: 0x2260},
		{from: 0x007d, to: 0x00a3},
		{from: 0x007e, to: 0x00b7},
	})
	msDecCyrillicCharset = newCharsetTable("DecCyrillic", 0xa0, 95, []charsetReplacement{
		{from: 0x00a1, to: 0x2426},
		{from: 0x00a2, to: 0x2426},
		{from: 0x00a3, to: 0x2426},
		{from: 0x00a4, to: 0x2426},
		{from: 0x00a5, to: 0x2426},
		{from: 0x00a6, to: 0x2426},
		{from: 0x00a7, to: 0x2426},
		{from: 0x00a8, to: 0x2426},
		{from: 0x00a9, to: 0x2426},
		{from: 0x00aa, to: 0x2426},
		{from: 0x00ab, to: 0x2426},
		{from: 0x00ac, to: 0x2426},
		{from: 0x00ad, to: 0x2426},
		{from: 0x00ae, to: 0x2426},
		{from: 0x00af, to: 0x2426},
		{from: 0x00b0, to: 0x2426},
		{from: 0x00b1, to: 0x2426},
		{from: 0x00b2, to: 0x2426},
		{from: 0x00b3, to: 0x2426},
		{from: 0x00b4, to: 0x2426},
		{from: 0x00b5, to: 0x2426},
		{from: 0x00b6, to: 0x2426},
		{from: 0x00b7, to: 0x2426},
		{from: 0x00b8, to: 0x2426},
		{from: 0x00b9, to: 0x2426},
		{from: 0x00ba, to: 0x2426},
		{from: 0x00bb, to: 0x2426},
		{from: 0x00bc, to: 0x2426},
		{from: 0x00bd, to: 0x2426},
		{from: 0x00be, to: 0x2426},
		{from: 0x00bf, to: 0x2426},
		{from: 0x00c0, to: 0x044e},
		{from: 0x00c1, to: 0x0430},
		{from: 0x00c2, to: 0x0431},
		{from: 0x00c3, to: 0x0446},
		{from: 0x00c4, to: 0x0434},
		{from: 0x00c5, to: 0x0435},
		{from: 0x00c6, to: 0x0444},
		{from: 0x00c7, to: 0x0433},
		{from: 0x00c8, to: 0x0445},
		{from: 0x00c9, to: 0x0438},
		{from: 0x00ca, to: 0x0439},
		{from: 0x00cb, to: 0x043a},
		{from: 0x00cc, to: 0x043b},
		{from: 0x00cd, to: 0x043c},
		{from: 0x00ce, to: 0x043d},
		{from: 0x00cf, to: 0x043e},
		{from: 0x00d0, to: 0x043f},
		{from: 0x00d1, to: 0x044f},
		{from: 0x00d2, to: 0x0440},
		{from: 0x00d3, to: 0x0441},
		{from: 0x00d4, to: 0x0442},
		{from: 0x00d5, to: 0x0443},
		{from: 0x00d6, to: 0x0436},
		{from: 0x00d7, to: 0x0432},
		{from: 0x00d8, to: 0x044c},
		{from: 0x00d9, to: 0x044b},
		{from: 0x00da, to: 0x0437},
		{from: 0x00db, to: 0x0448},
		{from: 0x00dc, to: 0x044d},
		{from: 0x00dd, to: 0x0449},
		{from: 0x00de, to: 0x0447},
		{from: 0x00df, to: 0x044a},
		{from: 0x00e0, to: 0x042e},
		{from: 0x00e1, to: 0x0410},
		{from: 0x00e2, to: 0x0411},
		{from: 0x00e3, to: 0x0426},
		{from: 0x00e4, to: 0x0414},
		{from: 0x00e5, to: 0x0415},
		{from: 0x00e6, to: 0x0424},
		{from: 0x00e7, to: 0x0413},
		{from: 0x00e8, to: 0x0425},
		{from: 0x00e9, to: 0x0418},
		{from: 0x00ea, to: 0x0419},
		{from: 0x00eb, to: 0x041a},
		{from: 0x00ec, to: 0x041b},
		{from: 0x00ed, to: 0x041c},
		{from: 0x00ee, to: 0x041d},
		{from: 0x00ef, to: 0x041e},
		{from: 0x00f0, to: 0x041f},
		{from: 0x00f1, to: 0x042f},
		{from: 0x00f2, to: 0x0420},
		{from: 0x00f3, to: 0x0421},
		{from: 0x00f4, to: 0x0422},
		{from: 0x00f5, to: 0x0423},
		{from: 0x00f6, to: 0x0416},
		{from: 0x00f7, to: 0x0412},
		{from: 0x00f8, to: 0x042c},
		{from: 0x00f9, to: 0x042b},
		{from: 0x00fa, to: 0x0417},
		{from: 0x00fb, to: 0x0428},
		{from: 0x00fc, to: 0x042d},
		{from: 0x00fd, to: 0x0429},
		{from: 0x00fe, to: 0x0427},
	})
	msDecGreekCharset = newCharsetTable("DecGreek", 0xa0, 95, []charsetReplacement{
		{from: 0x00a4, to: 0x2426},
		{from: 0x00a6, to: 0x2426},
		{from: 0x00a8, to: 0x00a4},
		{from: 0x00ac, to: 0x2426},
		{from: 0x00ad, to: 0x2426},
		{from: 0x00ae, to: 0x2426},
		{from: 0x00af, to: 0x2426},
		{from: 0x00b4, to: 0x2426},
		{from: 0x00b8, to: 0x2426},
		{from: 0x00be, to: 0x2426},
		{from: 0x00c0, to: 0x03ca},
		{from: 0x00c1, to: 0x0391},
		{from: 0x00c2, to: 0x0392},
		{from: 0x00c3, to: 0x0393},
		{from: 0x00c4, to: 0x0394},
		{from: 0x00c5, to: 0x0395},
		{from: 0x00c6, to: 0x0396},
		{from: 0x00c7, to: 0x0397},
		{from: 0x00c8, to: 0x0398},
		{from: 0x00c9, to: 0x0399},
		{from: 0x00ca, to: 0x039a},
		{from: 0x00cb, to: 0x039b},
		{from: 0x00cc, to: 0x039c},
		{from: 0x00cd, to: 0x039d},
		{from: 0x00ce, to: 0x039e},
		{from: 0x00cf, to: 0x039f},
		{from: 0x00d0, to: 0x2426},
		{from: 0x00d1, to: 0x03a0},
		{from: 0x00d2, to: 0x03a1},
		{from: 0x00d3, to: 0x03a3},
		{from: 0x00d4, to: 0x03a4},
		{from: 0x00d5, to: 0x03a5},
		{from: 0x00d6, to: 0x03a6},
		{from: 0x00d7, to: 0x03a7},
		{from: 0x00d8, to: 0x03a8},
		{from: 0x00d9, to: 0x03a9},
		{from: 0x00da, to: 0x03ac},
		{from: 0x00db, to: 0x03ad},
		{from: 0x00dc, to: 0x03ae},
		{from: 0x00dd, to: 0x03af},
		{from: 0x00de, to: 0x2426},
		{from: 0x00df, to: 0x03cc},
		{from: 0x00e0, to: 0x03cb},
		{from: 0x00e1, to: 0x03b1},
		{from: 0x00e2, to: 0x03b2},
		{from: 0x00e3, to: 0x03b3},
		{from: 0x00e4, to: 0x03b4},
		{from: 0x00e5, to: 0x03b5},
		{from: 0x00e6, to: 0x03b6},
		{from: 0x00e7, to: 0x03b7},
		{from: 0x00e8, to: 0x03b8},
		{from: 0x00e9, to: 0x03b9},
		{from: 0x00ea, to: 0x03ba},
		{from: 0x00eb, to: 0x03bb},
		{from: 0x00ec, to: 0x03bc},
		{from: 0x00ed, to: 0x03bd},
		{from: 0x00ee, to: 0x03be},
		{from: 0x00ef, to: 0x03bf},
		{from: 0x00f0, to: 0x2426},
		{from: 0x00f1, to: 0x03c0},
		{from: 0x00f2, to: 0x03c1},
		{from: 0x00f3, to: 0x03c3},
		{from: 0x00f4, to: 0x03c4},
		{from: 0x00f5, to: 0x03c5},
		{from: 0x00f6, to: 0x03c6},
		{from: 0x00f7, to: 0x03c7},
		{from: 0x00f8, to: 0x03c8},
		{from: 0x00f9, to: 0x03c9},
		{from: 0x00fa, to: 0x03c2},
		{from: 0x00fb, to: 0x03cd},
		{from: 0x00fc, to: 0x03ce},
		{from: 0x00fd, to: 0x0384},
		{from: 0x00fe, to: 0x2426},
	})
	msDecHebrewCharset = newCharsetTable("DecHebrew", 0xa0, 95, []charsetReplacement{
		{from: 0x00a4, to: 0x2426},
		{from: 0x00a6, to: 0x2426},
		{from: 0x00a8, to: 0x00a4},
		{from: 0x00ac, to: 0x2426},
		{from: 0x00ad, to: 0x2426},
		{from: 0x00ae, to: 0x2426},
		{from: 0x00af, to: 0x2426},
		{from: 0x00b4, to: 0x2426},
		{from: 0x00b8, to: 0x2426},
		{from: 0x00be, to: 0x2426},
		{from: 0x00c0, to: 0x2426},
		{from: 0x00c1, to: 0x2426},
		{from: 0x00c2, to: 0x2426},
		{from: 0x00c3, to: 0x2426},
		{from: 0x00c4, to: 0x2426},
		{from: 0x00c5, to: 0x2426},
		{from: 0x00c6, to: 0x2426},
		{from: 0x00c7, to: 0x2426},
		{from: 0x00c8, to: 0x2426},
		{from: 0x00c9, to: 0x2426},
		{from: 0x00ca, to: 0x2426},
		{from: 0x00cb, to: 0x2426},
		{from: 0x00cc, to: 0x2426},
		{from: 0x00cd, to: 0x2426},
		{from: 0x00ce, to: 0x2426},
		{from: 0x00cf, to: 0x2426},
		{from: 0x00d0, to: 0x2426},
		{from: 0x00d1, to: 0x2426},
		{from: 0x00d2, to: 0x2426},
		{from: 0x00d3, to: 0x2426},
		{from: 0x00d4, to: 0x2426},
		{from: 0x00d5, to: 0x2426},
		{from: 0x00d6, to: 0x2426},
		{from: 0x00d7, to: 0x2426},
		{from: 0x00d8, to: 0x2426},
		{from: 0x00d9, to: 0x2426},
		{from: 0x00da, to: 0x2426},
		{from: 0x00db, to: 0x2426},
		{from: 0x00dc, to: 0x2426},
		{from: 0x00dd, to: 0x2426},
		{from: 0x00de, to: 0x2426},
		{from: 0x00df, to: 0x2426},
		{from: 0x00e0, to: 0x05d0},
		{from: 0x00e1, to: 0x05d1},
		{from: 0x00e2, to: 0x05d2},
		{from: 0x00e3, to: 0x05d3},
		{from: 0x00e4, to: 0x05d4},
		{from: 0x00e5, to: 0x05d5},
		{from: 0x00e6, to: 0x05d6},
		{from: 0x00e7, to: 0x05d7},
		{from: 0x00e8, to: 0x05d8},
		{from: 0x00e9, to: 0x05d9},
		{from: 0x00ea, to: 0x05da},
		{from: 0x00eb, to: 0x05db},
		{from: 0x00ec, to: 0x05dc},
		{from: 0x00ed, to: 0x05dd},
		{from: 0x00ee, to: 0x05de},
		{from: 0x00ef, to: 0x05df},
		{from: 0x00f0, to: 0x05e0},
		{from: 0x00f1, to: 0x05e1},
		{from: 0x00f2, to: 0x05e2},
		{from: 0x00f3, to: 0x05e3},
		{from: 0x00f4, to: 0x05e4},
		{from: 0x00f5, to: 0x05e5},
		{from: 0x00f6, to: 0x05e6},
		{from: 0x00f7, to: 0x05e7},
		{from: 0x00f8, to: 0x05e8},
		{from: 0x00f9, to: 0x05e9},
		{from: 0x00fa, to: 0x05ea},
		{from: 0x00fb, to: 0x2426},
		{from: 0x00fc, to: 0x2426},
		{from: 0x00fd, to: 0x2426},
		{from: 0x00fe, to: 0x2426},
	})
	msDecTurkishCharset = newCharsetTable("DecTurkish", 0xa0, 95, []charsetReplacement{
		{from: 0x00a4, to: 0x2426},
		{from: 0x00a6, to: 0x2426},
		{from: 0x00a8, to: 0x00a4},
		{from: 0x00ac, to: 0x2426},
		{from: 0x00ad, to: 0x2426},
		{from: 0x00ae, to: 0x0130},
		{from: 0x00af, to: 0x2426},
		{from: 0x00b4, to: 0x2426},
		{from: 0x00b8, to: 0x2426},
		{from: 0x00be, to: 0x0131},
		{from: 0x00d0, to: 0x011e},
		{from: 0x00d7, to: 0x0152},
		{from: 0x00dd, to: 0x0178},
		{from: 0x00de, to: 0x015e},
		{from: 0x00f0, to: 0x011f},
		{from: 0x00f7, to: 0x0153},
		{from: 0x00fd, to: 0x00ff},
		{from: 0x00fe, to: 0x015f},
	})
	msBritishNrcsCharset = newCharsetTable("BritishNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00a3},
	})
	msDutchNrcsCharset = newCharsetTable("DutchNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00a3},
		{from: 0x0040, to: 0x00be},
		{from: 0x005b, to: 0x0133},
		{from: 0x005c, to: 0x00bd},
		{from: 0x005d, to: 0x007c},
		{from: 0x007b, to: 0x00a8},
		{from: 0x007c, to: 0x0192},
		{from: 0x007d, to: 0x00bc},
		{from: 0x007e, to: 0x00b4},
	})
	msFinnishNrcsCharset = newCharsetTable("FinnishNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x005b, to: 0x00c4},
		{from: 0x005c, to: 0x00d6},
		{from: 0x005d, to: 0x00c5},
		{from: 0x005e, to: 0x00dc},
		{from: 0x0060, to: 0x00e9},
		{from: 0x007b, to: 0x00e4},
		{from: 0x007c, to: 0x00f6},
		{from: 0x007d, to: 0x00e5},
		{from: 0x007e, to: 0x00fc},
	})
	msFrenchNrcsCharset = newCharsetTable("FrenchNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00a3},
		{from: 0x0040, to: 0x00e0},
		{from: 0x005b, to: 0x00b0},
		{from: 0x005c, to: 0x00e7},
		{from: 0x005d, to: 0x00a7},
		{from: 0x007b, to: 0x00e9},
		{from: 0x007c, to: 0x00f9},
		{from: 0x007d, to: 0x00e8},
		{from: 0x007e, to: 0x00a8},
	})
	msFrenchNrcsIsoCharset = newCharsetTable("FrenchNrcsIso", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00a3},
		{from: 0x0040, to: 0x00e0},
		{from: 0x005b, to: 0x00b0},
		{from: 0x005c, to: 0x00e7},
		{from: 0x005d, to: 0x00a7},
		{from: 0x0060, to: 0x00b5},
		{from: 0x007b, to: 0x00e9},
		{from: 0x007c, to: 0x00f9},
		{from: 0x007d, to: 0x00e8},
		{from: 0x007e, to: 0x00a8},
	})
	msFrenchCanadianNrcsCharset = newCharsetTable("FrenchCanadianNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0040, to: 0x00e0},
		{from: 0x005b, to: 0x00e2},
		{from: 0x005c, to: 0x00e7},
		{from: 0x005d, to: 0x00ea},
		{from: 0x005e, to: 0x00ee},
		{from: 0x0060, to: 0x00f4},
		{from: 0x007b, to: 0x00e9},
		{from: 0x007c, to: 0x00f9},
		{from: 0x007d, to: 0x00e8},
		{from: 0x007e, to: 0x00fb},
	})
	msGermanNrcsCharset = newCharsetTable("GermanNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0040, to: 0x00a7},
		{from: 0x005b, to: 0x00c4},
		{from: 0x005c, to: 0x00d6},
		{from: 0x005d, to: 0x00dc},
		{from: 0x007b, to: 0x00e4},
		{from: 0x007c, to: 0x00f6},
		{from: 0x007d, to: 0x00fc},
		{from: 0x007e, to: 0x00df},
	})
	msGreekNrcsCharset = newCharsetTable("GreekNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0040, to: 0x03ca},
		{from: 0x0041, to: 0x0391},
		{from: 0x0042, to: 0x0392},
		{from: 0x0043, to: 0x0393},
		{from: 0x0044, to: 0x0394},
		{from: 0x0045, to: 0x0395},
		{from: 0x0046, to: 0x0396},
		{from: 0x0047, to: 0x0397},
		{from: 0x0048, to: 0x0398},
		{from: 0x0049, to: 0x0399},
		{from: 0x004a, to: 0x039a},
		{from: 0x004b, to: 0x039b},
		{from: 0x004c, to: 0x039c},
		{from: 0x004d, to: 0x039d},
		{from: 0x004e, to: 0x039e},
		{from: 0x004f, to: 0x039f},
		{from: 0x0050, to: 0x2426},
		{from: 0x0051, to: 0x03a0},
		{from: 0x0052, to: 0x03a1},
		{from: 0x0053, to: 0x03a3},
		{from: 0x0054, to: 0x03a4},
		{from: 0x0055, to: 0x03a5},
		{from: 0x0056, to: 0x03a6},
		{from: 0x0057, to: 0x03a7},
		{from: 0x0058, to: 0x03a8},
		{from: 0x0059, to: 0x03a9},
		{from: 0x005a, to: 0x03ac},
		{from: 0x005b, to: 0x03ad},
		{from: 0x005c, to: 0x03ae},
		{from: 0x005d, to: 0x03af},
		{from: 0x005e, to: 0x2426},
		{from: 0x005f, to: 0x03cc},
		{from: 0x0060, to: 0x03cb},
		{from: 0x0061, to: 0x03b1},
		{from: 0x0062, to: 0x03b2},
		{from: 0x0063, to: 0x03b3},
		{from: 0x0064, to: 0x03b4},
		{from: 0x0065, to: 0x03b5},
		{from: 0x0066, to: 0x03b6},
		{from: 0x0067, to: 0x03b7},
		{from: 0x0068, to: 0x03b8},
		{from: 0x0069, to: 0x03b9},
		{from: 0x006a, to: 0x03ba},
		{from: 0x006b, to: 0x03bb},
		{from: 0x006c, to: 0x03bc},
		{from: 0x006d, to: 0x03bd},
		{from: 0x006e, to: 0x03be},
		{from: 0x006f, to: 0x03bf},
		{from: 0x0070, to: 0x2426},
		{from: 0x0071, to: 0x03c0},
		{from: 0x0072, to: 0x03c1},
		{from: 0x0073, to: 0x03c3},
		{from: 0x0074, to: 0x03c4},
		{from: 0x0075, to: 0x03c5},
		{from: 0x0076, to: 0x03c6},
		{from: 0x0077, to: 0x03c7},
		{from: 0x0078, to: 0x03c8},
		{from: 0x0079, to: 0x03c9},
		{from: 0x007a, to: 0x03c2},
		{from: 0x007b, to: 0x03cd},
		{from: 0x007c, to: 0x03ce},
		{from: 0x007d, to: 0x0384},
		{from: 0x007e, to: 0x2426},
	})
	msHebrewNrcsCharset = newCharsetTable("HebrewNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0060, to: 0x05d0},
		{from: 0x0061, to: 0x05d1},
		{from: 0x0062, to: 0x05d2},
		{from: 0x0063, to: 0x05d3},
		{from: 0x0064, to: 0x05d4},
		{from: 0x0065, to: 0x05d5},
		{from: 0x0066, to: 0x05d6},
		{from: 0x0067, to: 0x05d7},
		{from: 0x0068, to: 0x05d8},
		{from: 0x0069, to: 0x05d9},
		{from: 0x006a, to: 0x05da},
		{from: 0x006b, to: 0x05db},
		{from: 0x006c, to: 0x05dc},
		{from: 0x006d, to: 0x05dd},
		{from: 0x006e, to: 0x05de},
		{from: 0x006f, to: 0x05df},
		{from: 0x0070, to: 0x05e0},
		{from: 0x0071, to: 0x05e1},
		{from: 0x0072, to: 0x05e2},
		{from: 0x0073, to: 0x05e3},
		{from: 0x0074, to: 0x05e4},
		{from: 0x0075, to: 0x05e5},
		{from: 0x0076, to: 0x05e6},
		{from: 0x0077, to: 0x05e7},
		{from: 0x0078, to: 0x05e8},
		{from: 0x0079, to: 0x05e9},
		{from: 0x007a, to: 0x05ea},
	})
	msItalianNrcsCharset = newCharsetTable("ItalianNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00a3},
		{from: 0x0040, to: 0x00a7},
		{from: 0x005b, to: 0x00b0},
		{from: 0x005c, to: 0x00e7},
		{from: 0x005d, to: 0x00e9},
		{from: 0x0060, to: 0x00f9},
		{from: 0x007b, to: 0x00e0},
		{from: 0x007c, to: 0x00f2},
		{from: 0x007d, to: 0x00e8},
		{from: 0x007e, to: 0x00ec},
	})
	msNorwegianDanishNrcsCharset = newCharsetTable("NorwegianDanishNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0040, to: 0x00c4},
		{from: 0x005b, to: 0x00c6},
		{from: 0x005c, to: 0x00d8},
		{from: 0x005d, to: 0x00c5},
		{from: 0x005e, to: 0x00dc},
		{from: 0x0060, to: 0x00e4},
		{from: 0x007b, to: 0x00e6},
		{from: 0x007c, to: 0x00f8},
		{from: 0x007d, to: 0x00e5},
		{from: 0x007e, to: 0x00fc},
	})
	msNorwegianDanishNrcsIsoCharset = newCharsetTable("NorwegianDanishNrcsIso", 0x20, 95, []charsetReplacement{
		{from: 0x005b, to: 0x00c6},
		{from: 0x005c, to: 0x00d8},
		{from: 0x005d, to: 0x00c5},
		{from: 0x007b, to: 0x00e6},
		{from: 0x007c, to: 0x00f8},
		{from: 0x007d, to: 0x00e5},
	})
	msPortugueseNrcsCharset = newCharsetTable("PortugueseNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x005b, to: 0x00c3},
		{from: 0x005c, to: 0x00c7},
		{from: 0x005d, to: 0x00d5},
		{from: 0x007b, to: 0x00e3},
		{from: 0x007c, to: 0x00e7},
		{from: 0x007d, to: 0x00f5},
	})
	msRussianNrcsCharset = newCharsetTable("RussianNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0060, to: 0x042e},
		{from: 0x0061, to: 0x0410},
		{from: 0x0062, to: 0x0411},
		{from: 0x0063, to: 0x0426},
		{from: 0x0064, to: 0x0414},
		{from: 0x0065, to: 0x0415},
		{from: 0x0066, to: 0x0424},
		{from: 0x0067, to: 0x0413},
		{from: 0x0068, to: 0x0425},
		{from: 0x0069, to: 0x0418},
		{from: 0x006a, to: 0x0419},
		{from: 0x006b, to: 0x041a},
		{from: 0x006c, to: 0x041b},
		{from: 0x006d, to: 0x041c},
		{from: 0x006e, to: 0x041d},
		{from: 0x006f, to: 0x041e},
		{from: 0x0070, to: 0x041f},
		{from: 0x0071, to: 0x042f},
		{from: 0x0072, to: 0x0420},
		{from: 0x0073, to: 0x0421},
		{from: 0x0074, to: 0x0422},
		{from: 0x0075, to: 0x0423},
		{from: 0x0076, to: 0x0416},
		{from: 0x0077, to: 0x0412},
		{from: 0x0078, to: 0x042c},
		{from: 0x0079, to: 0x042b},
		{from: 0x007a, to: 0x0417},
		{from: 0x007b, to: 0x0428},
		{from: 0x007c, to: 0x042d},
		{from: 0x007d, to: 0x0429},
		{from: 0x007e, to: 0x0427},
	})
	msSpanishNrcsCharset = newCharsetTable("SpanishNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00a3},
		{from: 0x0040, to: 0x00a7},
		{from: 0x005b, to: 0x00a1},
		{from: 0x005c, to: 0x00d1},
		{from: 0x005d, to: 0x00bf},
		{from: 0x007b, to: 0x00b0},
		{from: 0x007c, to: 0x00f1},
		{from: 0x007d, to: 0x00e7},
	})
	msSwedishNrcsCharset = newCharsetTable("SwedishNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0040, to: 0x00c9},
		{from: 0x005b, to: 0x00c4},
		{from: 0x005c, to: 0x00d6},
		{from: 0x005d, to: 0x00c5},
		{from: 0x005e, to: 0x00dc},
		{from: 0x0060, to: 0x00e9},
		{from: 0x007b, to: 0x00e4},
		{from: 0x007c, to: 0x00f6},
		{from: 0x007d, to: 0x00e5},
		{from: 0x007e, to: 0x00fc},
	})
	msSwissNrcsCharset = newCharsetTable("SwissNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0023, to: 0x00f9},
		{from: 0x0040, to: 0x00e0},
		{from: 0x005b, to: 0x00e9},
		{from: 0x005c, to: 0x00e7},
		{from: 0x005d, to: 0x00ea},
		{from: 0x005e, to: 0x00ee},
		{from: 0x005f, to: 0x00e8},
		{from: 0x0060, to: 0x00f4},
		{from: 0x007b, to: 0x00e4},
		{from: 0x007c, to: 0x00f6},
		{from: 0x007d, to: 0x00fc},
		{from: 0x007e, to: 0x00fb},
	})
	msTurkishNrcsCharset = newCharsetTable("TurkishNrcs", 0x20, 95, []charsetReplacement{
		{from: 0x0021, to: 0x0131},
		{from: 0x0026, to: 0x011f},
		{from: 0x0040, to: 0x0130},
		{from: 0x005b, to: 0x015e},
		{from: 0x005c, to: 0x00d6},
		{from: 0x005d, to: 0x00c7},
		{from: 0x005e, to: 0x00dc},
		{from: 0x0060, to: 0x011e},
		{from: 0x007b, to: 0x015f},
		{from: 0x007c, to: 0x00f6},
		{from: 0x007d, to: 0x00e7},
		{from: 0x007e, to: 0x00fc},
	})
	msDrcs94Charset = newCharsetTable("Drcs94", 0xef20, 95, []charsetReplacement{{from: 0xef20, to: 0x20}})
	msDrcs96Charset = newCharsetTable("Drcs96", 0xef20, 96, nil)
	msCharsetByName = map[string]*charsetTable{
		"B": msASCIICharset, "1": msASCIICharset,
		"0": msDecSpecialGraphicsCharset, "2": msDecSpecialGraphicsCharset,
		"<": msDecSupplementalCharset, "A": msBritishNrcsCharset,
		"4": msDutchNrcsCharset, "5": msFinnishNrcsCharset,
		"C": msFinnishNrcsCharset, "R": msFrenchNrcsCharset, "f": msFrenchNrcsIsoCharset,
		"9": msFrenchCanadianNrcsCharset, "Q": msFrenchCanadianNrcsCharset,
		"K": msGermanNrcsCharset, "Y": msItalianNrcsCharset,
		"6": msNorwegianDanishNrcsCharset, "E": msNorwegianDanishNrcsCharset,
		"`": msNorwegianDanishNrcsIsoCharset, "Z": msSpanishNrcsCharset,
		"7": msSwedishNrcsCharset, "H": msSwedishNrcsCharset, "=": msSwissNrcsCharset,
		"&4": msDecCyrillicCharset, "&5": msRussianNrcsCharset,
		"\"?": msDecGreekCharset, "\">": msGreekNrcsCharset, "\"4": msDecHebrewCharset,
		"%=": msHebrewNrcsCharset, "%0": msDecTurkishCharset, "%2": msTurkishNrcsCharset,
		"%5": msDecSupplementalCharset, "%6": msPortugueseNrcsCharset,
	}
	msCharset96ByName = map[string]*charsetTable{
		"A": msLatin1Charset, "<": msLatin1Charset, "B": msLatin2Charset,
		"L": msLatinCyrillicCharset, "F": msLatinGreekCharset, "H": msLatinHebrewCharset, "M": msLatin5Charset,
	}
)

// terminalOutput is TerminalOutput. A nil active table is the source empty
// std::wstring_view used when no translation is needed.
type terminalOutput struct {
	gsets                [4]*charsetTable
	glSetNumber          int
	grSetNumber          int
	glTranslationTable   *charsetTable
	grTranslationTable   *charsetTable
	ssTranslationTable   *charsetTable
	grTranslationEnabled bool
	drcsID               string
	drcsTranslationTable *charsetTable
}

func newTerminalOutput() terminalOutput {
	return terminalOutput{gsets: [4]*charsetTable{msASCIICharset, msASCIICharset, msASCIICharset, msASCIICharset}, glSetNumber: 0, grSetNumber: 2}
}

func (t *terminalOutput) designate94Charset(gset int, id string) bool {
	table := t.lookup94(id)
	if table == nil || gset < 0 || gset >= len(t.gsets) {
		return false
	}
	return t.setTranslationTable(gset, table)
}

func (t *terminalOutput) designate96Charset(gset int, id string) bool {
	table := t.lookup96(id)
	if table == nil || gset < 0 || gset >= len(t.gsets) {
		return false
	}
	return t.setTranslationTable(gset, table)
}

func (t *terminalOutput) setDrcs94Designation(id string) {
	t.replaceDrcsTable(t.lookup94(id), msDrcs94Charset)
	t.drcsID = id
	t.drcsTranslationTable = msDrcs94Charset
}

func (t *terminalOutput) setDrcs96Designation(id string) {
	t.replaceDrcsTable(t.lookup96(id), msDrcs96Charset)
	t.drcsID = id
	t.drcsTranslationTable = msDrcs96Charset
}

func (t *terminalOutput) lockingShift(gset int) bool {
	if gset < 0 || gset >= len(t.gsets) {
		return false
	}
	t.glSetNumber = gset
	t.glTranslationTable = t.gsets[gset]
	if t.glTranslationTable == msASCIICharset {
		t.glTranslationTable = nil
	}
	return true
}

func (t *terminalOutput) lockingShiftRight(gset int) bool {
	if gset < 0 || gset >= len(t.gsets) {
		return false
	}
	t.grSetNumber = gset
	t.grTranslationTable = t.gsets[gset]
	if t.grTranslationTable == msLatin1Charset || !t.grTranslationEnabled {
		t.grTranslationTable = nil
	}
	return true
}

func (t *terminalOutput) singleShift(gset int) bool {
	if gset < 0 || gset >= len(t.gsets) {
		return false
	}
	t.ssTranslationTable = t.gsets[gset]
	return true
}

func (t *terminalOutput) needToTranslate() bool {
	return t.glTranslationTable != nil || t.grTranslationTable != nil || t.ssTranslationTable != nil
}

func (t *terminalOutput) enableGrTranslation(enabled bool) {
	t.grTranslationEnabled = enabled
	defaultTable := msASCIICharset
	if enabled {
		defaultTable = msLatin1Charset
	}
	t.gsets[2] = defaultTable
	t.gsets[3] = defaultTable
	t.lockingShift(t.glSetNumber)
	t.lockingShiftRight(t.grSetNumber)
}

func (t *terminalOutput) translateKey(wch uint16) uint16 {
	if t.ssTranslationTable != nil {
		if wch >= 0x20 && uint32(wch-0x20) < uint32(t.ssTranslationTable.size) {
			wch = t.ssTranslationTable.translate(wch - 0x20)
		} else if wch >= 0xa0 && uint32(wch-0xa0) < uint32(t.ssTranslationTable.size) {
			wch = t.ssTranslationTable.translate(wch - 0xa0)
		}
		t.ssTranslationTable = nil
	} else if t.glTranslationTable != nil {
		if wch >= 0x20 && uint32(wch-0x20) < uint32(t.glTranslationTable.size) {
			wch = t.glTranslationTable.translate(wch - 0x20)
		} else if t.grTranslationTable != nil && wch >= 0xa0 && uint32(wch-0xa0) < uint32(t.grTranslationTable.size) {
			wch = t.grTranslationTable.translate(wch - 0xa0)
		}
	} else if t.grTranslationTable != nil && wch >= 0xa0 && uint32(wch-0xa0) < uint32(t.grTranslationTable.size) {
		wch = t.grTranslationTable.translate(wch - 0xa0)
	}
	return wch
}

func (t *terminalOutput) lookup94(id string) *charsetTable {
	if id == t.drcsID {
		return t.drcsTranslationTable
	}
	return msCharsetByName[id]
}

func (t *terminalOutput) lookup96(id string) *charsetTable {
	if id == t.drcsID {
		return t.drcsTranslationTable
	}
	return msCharset96ByName[id]
}

// lookup94Value and lookup96Value consume the VTID value produced by
// DispatchTypes::VTIDBuilder. The pinned adapter passes the command
// parameter as a VTID, not as a length-delimited string.
func (t *terminalOutput) lookup94Value(id uint64) *charsetTable {
	if t.drcsID != "" && id == vtIDFromString(t.drcsID) {
		return t.drcsTranslationTable
	}
	for name, table := range msCharsetByName {
		if id == vtIDFromString(name) {
			return table
		}
	}
	return nil
}

func (t *terminalOutput) lookup96Value(id uint64) *charsetTable {
	if t.drcsID != "" && id == vtIDFromString(t.drcsID) {
		return t.drcsTranslationTable
	}
	for name, table := range msCharset96ByName {
		if id == vtIDFromString(name) {
			return table
		}
	}
	return nil
}

func (t *terminalOutput) designate94CharsetValue(gset int, id uint64) bool {
	table := t.lookup94Value(id)
	if table == nil || gset < 0 || gset >= len(t.gsets) {
		return false
	}
	return t.setTranslationTable(gset, table)
}

func (t *terminalOutput) designate96CharsetValue(gset int, id uint64) bool {
	table := t.lookup96Value(id)
	if table == nil || gset < 0 || gset >= len(t.gsets) {
		return false
	}
	return t.setTranslationTable(gset, table)
}

func (t *terminalOutput) setTranslationTable(gset int, table *charsetTable) bool {
	t.gsets[gset] = table
	return t.lockingShift(t.glSetNumber) && t.lockingShiftRight(t.grSetNumber)
}

func (t *terminalOutput) replaceDrcsTable(oldTable, newTable *charsetTable) {
	if oldTable == newTable {
		return
	}
	for gset := range t.gsets {
		table := t.gsets[gset]
		if table == msDrcs94Charset || table == msDrcs96Charset {
			if gset < 2 {
				table = msASCIICharset
			} else {
				table = msLatin1Charset
			}
		}
		if table == oldTable {
			table = newTable
		}
		t.gsets[gset] = table
	}
	t.lockingShift(t.glSetNumber)
	t.lockingShiftRight(t.grSetNumber)
}
