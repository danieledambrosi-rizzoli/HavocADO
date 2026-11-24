
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:1

package hclsyntax

// This file is generated from scan_string_lit.rl. DO NOT EDIT.

//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:7
const hclstrtok_start int = 4
const hclstrtok_first_final int = 4
const hclstrtok_error int = 0

const hclstrtok_en_quoted int = 10
const hclstrtok_en_unquoted int = 4


//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:10


func scanStringLit(data []byte, quoted bool) [][]byte {
    var ret [][]byte

    
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:61


    // Ragel state
	p := 0  // "Pointer" into data
	pe := len(data) // End-of-data "pointer"
    ts := 0
    te := 0
    eof := pe

    var cs int // current state
    switch {
    case quoted:
        cs = hclstrtok_en_quoted
    default:
        cs = hclstrtok_en_unquoted
    }

    // Make Go compiler happy
    _ = ts
    _ = eof

    /*token := func () {
        ret = append(ret, data[ts:te])
    }*/

    
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:47
	{
	}

//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:50
	{
	if p == pe {
		goto _test_eof
	}
	switch cs {
	case 4:
		goto st_case_4
	case 5:
		goto st_case_5
	case 6:
		goto st_case_6
	case 7:
		goto st_case_7
	case 8:
		goto st_case_8
	case 9:
		goto st_case_9
	case 10:
		goto st_case_10
	case 11:
		goto st_case_11
	case 12:
		goto st_case_12
	case 13:
		goto st_case_13
	case 14:
		goto st_case_14
	case 15:
		goto st_case_15
	case 16:
		goto st_case_16
	case 17:
		goto st_case_17
	case 18:
		goto st_case_18
	case 19:
		goto st_case_19
	case 20:
		goto st_case_20
	case 21:
		goto st_case_21
	case 22:
		goto st_case_22
	case 23:
		goto st_case_23
	case 24:
		goto st_case_24
	case 1:
		goto st_case_1
	case 0:
		goto st_case_0
	case 2:
		goto st_case_2
	case 3:
		goto st_case_3
	}
	goto st_out
tr9:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
	goto st4
	st4:
		if p++; p == pe {
			goto _test_eof4
		}
	st_case_4:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:120
		switch data[p] {
		case 10:
			goto tr5
		case 13:
			goto tr6
		case 36:
			goto tr7
		case 37:
			goto tr8
		}
		goto st4
tr5:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st5
tr10:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st5
	st5:
		if p++; p == pe {
			goto _test_eof5
		}
	st_case_5:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:164
		switch data[p] {
		case 10:
			goto tr10
		case 13:
			goto tr11
		case 36:
			goto tr12
		case 37:
			goto tr13
		}
		goto tr9
tr6:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st6
tr11:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st6
	st6:
		if p++; p == pe {
			goto _test_eof6
		}
	st_case_6:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:208
		switch data[p] {
		case 10:
			goto st5
		case 13:
			goto tr11
		case 36:
			goto tr12
		case 37:
			goto tr13
		}
		goto tr9
tr7:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st7
tr12:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st7
	st7:
		if p++; p == pe {
			goto _test_eof7
		}
	st_case_7:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:252
		switch data[p] {
		case 10:
			goto tr10
		case 13:
			goto tr11
		case 36:
			goto st8
		case 37:
			goto tr13
		}
		goto tr9
	st8:
		if p++; p == pe {
			goto _test_eof8
		}
	st_case_8:
		switch data[p] {
		case 10:
			goto tr10
		case 13:
			goto tr11
		case 36:
			goto tr12
		case 37:
			goto tr13
		case 123:
			goto st5
		}
		goto tr9
tr8:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st9
tr13:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st9
	st9:
		if p++; p == pe {
			goto _test_eof9
		}
	st_case_9:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:314
		switch data[p] {
		case 10:
			goto tr10
		case 13:
			goto tr11
		case 36:
			goto tr12
		case 37:
			goto st8
		}
		goto tr9
tr22:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
	goto st10
	st10:
		if p++; p == pe {
			goto _test_eof10
		}
	st_case_10:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:338
		switch data[p] {
		case 10:
			goto tr17
		case 13:
			goto tr18
		case 36:
			goto tr19
		case 37:
			goto tr20
		case 92:
			goto tr21
		}
		goto st10
tr17:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st11
tr23:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st11
	st11:
		if p++; p == pe {
			goto _test_eof11
		}
	st_case_11:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:384
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		goto tr22
tr18:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st12
tr24:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st12
	st12:
		if p++; p == pe {
			goto _test_eof12
		}
	st_case_12:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:430
		switch data[p] {
		case 10:
			goto st11
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		goto tr22
tr19:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st13
tr25:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st13
	st13:
		if p++; p == pe {
			goto _test_eof13
		}
	st_case_13:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:476
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto st14
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		goto tr22
	st14:
		if p++; p == pe {
			goto _test_eof14
		}
	st_case_14:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		case 123:
			goto st11
		}
		goto tr22
tr20:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st15
tr26:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st15
	st15:
		if p++; p == pe {
			goto _test_eof15
		}
	st_case_15:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:542
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto st14
		case 92:
			goto tr27
		}
		goto tr22
tr21:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st16
tr27:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:40

            // If te is behind p then we've skipped over some literal
            // characters which we must now return.
            if te < p {
                ret = append(ret, data[te:p])
            }
            ts = p;
        
	goto st16
	st16:
		if p++; p == pe {
			goto _test_eof16
		}
	st_case_16:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:588
		switch data[p] {
		case 85:
			goto st17
		case 117:
			goto st21
		}
		switch {
		case data[p] < 224:
			switch {
			case data[p] > 191:
				if 192 <= data[p] && data[p] <= 223 {
					goto st1
				}
			case data[p] >= 128:
				goto tr22
			}
		case data[p] > 239:
			switch {
			case data[p] > 247:
				if 248 <= data[p] {
					goto tr22
				}
			case data[p] >= 240:
				goto st3
			}
		default:
			goto st2
		}
		goto st11
	st17:
		if p++; p == pe {
			goto _test_eof17
		}
	st_case_17:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st18
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st18
			}
		default:
			goto st18
		}
		goto tr22
	st18:
		if p++; p == pe {
			goto _test_eof18
		}
	st_case_18:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st19
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st19
			}
		default:
			goto st19
		}
		goto tr22
	st19:
		if p++; p == pe {
			goto _test_eof19
		}
	st_case_19:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st20
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st20
			}
		default:
			goto st20
		}
		goto tr22
	st20:
		if p++; p == pe {
			goto _test_eof20
		}
	st_case_20:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st21
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st21
			}
		default:
			goto st21
		}
		goto tr22
	st21:
		if p++; p == pe {
			goto _test_eof21
		}
	st_case_21:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st22
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st22
			}
		default:
			goto st22
		}
		goto tr22
	st22:
		if p++; p == pe {
			goto _test_eof22
		}
	st_case_22:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st23
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st23
			}
		default:
			goto st23
		}
		goto tr22
	st23:
		if p++; p == pe {
			goto _test_eof23
		}
	st_case_23:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st24
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st24
			}
		default:
			goto st24
		}
		goto tr22
	st24:
		if p++; p == pe {
			goto _test_eof24
		}
	st_case_24:
		switch data[p] {
		case 10:
			goto tr23
		case 13:
			goto tr24
		case 36:
			goto tr25
		case 37:
			goto tr26
		case 92:
			goto tr27
		}
		switch {
		case data[p] < 65:
			if 48 <= data[p] && data[p] <= 57 {
				goto st11
			}
		case data[p] > 70:
			if 97 <= data[p] && data[p] <= 102 {
				goto st11
			}
		default:
			goto st11
		}
		goto tr22
	st1:
		if p++; p == pe {
			goto _test_eof1
		}
	st_case_1:
		if 128 <= data[p] && data[p] <= 191 {
			goto st11
		}
		goto st0
st_case_0:
	st0:
		cs = 0
		goto _out
	st2:
		if p++; p == pe {
			goto _test_eof2
		}
	st_case_2:
		if 128 <= data[p] && data[p] <= 191 {
			goto st1
		}
		goto st0
	st3:
		if p++; p == pe {
			goto _test_eof3
		}
	st_case_3:
		if 128 <= data[p] && data[p] <= 191 {
			goto st2
		}
		goto st0
	st_out:
	_test_eof4: cs = 4; goto _test_eof
	_test_eof5: cs = 5; goto _test_eof
	_test_eof6: cs = 6; goto _test_eof
	_test_eof7: cs = 7; goto _test_eof
	_test_eof8: cs = 8; goto _test_eof
	_test_eof9: cs = 9; goto _test_eof
	_test_eof10: cs = 10; goto _test_eof
	_test_eof11: cs = 11; goto _test_eof
	_test_eof12: cs = 12; goto _test_eof
	_test_eof13: cs = 13; goto _test_eof
	_test_eof14: cs = 14; goto _test_eof
	_test_eof15: cs = 15; goto _test_eof
	_test_eof16: cs = 16; goto _test_eof
	_test_eof17: cs = 17; goto _test_eof
	_test_eof18: cs = 18; goto _test_eof
	_test_eof19: cs = 19; goto _test_eof
	_test_eof20: cs = 20; goto _test_eof
	_test_eof21: cs = 21; goto _test_eof
	_test_eof22: cs = 22; goto _test_eof
	_test_eof23: cs = 23; goto _test_eof
	_test_eof24: cs = 24; goto _test_eof
	_test_eof1: cs = 1; goto _test_eof
	_test_eof2: cs = 2; goto _test_eof
	_test_eof3: cs = 3; goto _test_eof

	_test_eof: {}
	if p == eof {
		switch cs {
		case 5, 6, 7, 8, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24:
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:48

            te = p;
            ret = append(ret, data[ts:te]);
        
//line pkg/profile/yaotl/hclsyntax/scan_string_lit.go:924
		}
	}

	_out: {}
	}

//line pkg/profile/yaotl/hclsyntax/scan_string_lit.rl:89


    if te < p {
        // Collect any leftover literal characters at the end of the input
        ret = append(ret, data[te:p])
    }

    // If we fall out here without being in a final state then we've
    // encountered something that the scanner can't match, which should
    // be impossible (the scanner matches all bytes _somehow_) but we'll
    // tolerate it and let the caller deal with it.
    if cs < hclstrtok_first_final {
        ret = append(ret, data[p:len(data)])
    }

    return ret
}
