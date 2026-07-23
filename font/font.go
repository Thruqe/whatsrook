// Font styling engine – converts text to various decorative Unicode/ASCII styles.
package font

import (
	"strings"
	"sync"
)

var (
	currentStyle = "monospace"
	mu           sync.RWMutex
)

// SetStyle sets the active font style for text conversion.
func SetStyle(style string) {
	mu.Lock()
	defer mu.Unlock()
	currentStyle = strings.ToLower(style)
}

// GetStyle returns the currently active font style name.
func GetStyle() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentStyle
}

// Convert transforms the input string to the currently active font style.
func Convert(s string) string {
	style := GetStyle()
	var sb strings.Builder
	for _, r := range s {
		switch style {
		case "monospace":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D68A)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D670)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7F6)
			} else {
				sb.WriteRune(r)
			}
		case "bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "italic":
			if r >= 'a' && r <= 'z' {
				if r == 'h' {
					sb.WriteRune(0x0210E)
				} else {
					sb.WriteRune(r - 'a' + 0x1D434 + 26)
				}
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D434)
			} else {
				sb.WriteRune(r)
			}
		case "bold-italic":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D482)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D468)
			} else {
				sb.WriteRune(r)
			}
		case "double-struck":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D552)
			} else if r >= 'A' && r <= 'Z' {
				switch r {
				case 'C':
					sb.WriteRune(0x2102)
				case 'H':
					sb.WriteRune(0x210D)
				case 'N':
					sb.WriteRune(0x2115)
				case 'P':
					sb.WriteRune(0x2119)
				case 'Q':
					sb.WriteRune(0x211A)
				case 'R':
					sb.WriteRune(0x211D)
				case 'Z':
					sb.WriteRune(0x2124)
				default:
					sb.WriteRune(r - 'A' + 0x1D538)
				}
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7D8)
			} else {
				sb.WriteRune(r)
			}
		case "script":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4EA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D4D0)
			} else {
				sb.WriteRune(r)
			}
		case "bold-script":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4B6)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D49C)
			} else {
				sb.WriteRune(r)
			}
		case "fraktur":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D520)
			} else if r >= 'A' && r <= 'Z' {
				switch r {
				case 'C':
					sb.WriteRune(0x212C)
				case 'H':
					sb.WriteRune(0x210C)
				case 'I':
					sb.WriteRune(0x2111)
				case 'R':
					sb.WriteRune(0x211C)
				case 'Z':
					sb.WriteRune(0x2128)
				default:
					sb.WriteRune(r - 'A' + 0x1D504)
				}
			} else {
				sb.WriteRune(r)
			}
		case "bold-fraktur":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D586)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D56C)
			} else {
				sb.WriteRune(r)
			}
		case "sans":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D586 - 52)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0 - 52)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7E2)
			} else {
				sb.WriteRune(r)
			}
		case "sans-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "sans-italic":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D608)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5EE)
			} else {
				sb.WriteRune(r)
			}
		case "sans-bold-italic":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D63C)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D622)
			} else {
				sb.WriteRune(r)
			}
		case "circled":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x24D0)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x24B6)
			} else if r >= '1' && r <= '9' {
				sb.WriteRune(r - '1' + 0x2460)
			} else if r == '0' {
				sb.WriteRune(0x24EA)
			} else {
				sb.WriteRune(r)
			}
		case "circled-negative":
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				if r >= 'a' && r <= 'z' {
					r -= 'a' - 'A'
				}
				sb.WriteRune(r - 'A' + 0x1F150)
			} else if r >= '1' && r <= '9' {
				sb.WriteRune(r - '1' + 0x2776)
			} else if r == '0' {
				sb.WriteRune(0x24FF)
			} else {
				sb.WriteRune(r)
			}
		case "squared":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1F130)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1F130)
			} else {
				sb.WriteRune(r)
			}
		case "squared-negative":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1F170)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1F170)
			} else {
				sb.WriteRune(r)
			}
		case "fullwidth":
			if r >= '!' && r <= '~' {
				sb.WriteRune(r - '!' + 0xFF01)
			} else if r == ' ' {
				sb.WriteRune(0x3000)
			} else {
				sb.WriteRune(r)
			}
		case "small-caps":
			switch r {
			case 'a':
				sb.WriteRune(0x1D00)
			case 'b':
				sb.WriteRune(0x0299)
			case 'c':
				sb.WriteRune(0x1D04)
			case 'd':
				sb.WriteRune(0x1D05)
			case 'e':
				sb.WriteRune(0x1D07)
			case 'f':
				sb.WriteRune(0xA730)
			case 'g':
				sb.WriteRune(0x0262)
			case 'h':
				sb.WriteRune(0x029C)
			case 'i':
				sb.WriteRune(0x026A)
			case 'j':
				sb.WriteRune(0x1D0A)
			case 'k':
				sb.WriteRune(0x1D0B)
			case 'l':
				sb.WriteRune(0x029F)
			case 'm':
				sb.WriteRune(0x1D0D)
			case 'n':
				sb.WriteRune(0x0274)
			case 'o':
				sb.WriteRune(0x1D0F)
			case 'p':
				sb.WriteRune(0x1D18)
			case 'q':
				sb.WriteRune(0x01AA)
			case 'r':
				sb.WriteRune(0x0280)
			case 's':
				sb.WriteRune(0x01A1)
			case 't':
				sb.WriteRune(0x1D1B)
			case 'u':
				sb.WriteRune(0x1D1C)
			case 'v':
				sb.WriteRune(0x1D20)
			case 'w':
				sb.WriteRune(0x1D21)
			case 'x':
				sb.WriteRune('x')
			case 'y':
				sb.WriteRune(0x028F)
			case 'z':
				sb.WriteRune(0x1D22)
			default:
				sb.WriteRune(r)
			}
		case "subscript":
			switch r {
			case '0':
				sb.WriteRune(0x2080)
			case '1':
				sb.WriteRune(0x2081)
			case '2':
				sb.WriteRune(0x2082)
			case '3':
				sb.WriteRune(0x2083)
			case '4':
				sb.WriteRune(0x2084)
			case '5':
				sb.WriteRune(0x2085)
			case '6':
				sb.WriteRune(0x2086)
			case '7':
				sb.WriteRune(0x2087)
			case '8':
				sb.WriteRune(0x2088)
			case '9':
				sb.WriteRune(0x2089)
			case 'a':
				sb.WriteRune(0x2090)
			case 'e':
				sb.WriteRune(0x2095)
			case 'h':
				sb.WriteRune(0x2096)
			case 'i':
				sb.WriteRune(0x1D62)
			case 'k':
				sb.WriteRune(0x2097)
			case 'l':
				sb.WriteRune(0x2098)
			case 'm':
				sb.WriteRune(0x2099)
			case 'n':
				sb.WriteRune(0x209A)
			case 'o':
				sb.WriteRune(0x2092)
			case 'p':
				sb.WriteRune(0x209B)
			case 'r':
				sb.WriteRune(0x1D63)
			case 's':
				sb.WriteRune(0x209C)
			case 't':
				sb.WriteRune(0x209D)
			case 'u':
				sb.WriteRune(0x1D64)
			case 'v':
				sb.WriteRune(0x1D65)
			case 'x':
				sb.WriteRune(0x2088)
			default:
				sb.WriteRune(r)
			}
		case "superscript":
			switch r {
			case '0':
				sb.WriteRune(0x2070)
			case '1':
				sb.WriteRune(0x00B9)
			case '2':
				sb.WriteRune(0x00B2)
			case '3':
				sb.WriteRune(0x00B3)
			case '4':
				sb.WriteRune(0x2074)
			case '5':
				sb.WriteRune(0x2075)
			case '6':
				sb.WriteRune(0x2076)
			case '7':
				sb.WriteRune(0x2077)
			case '8':
				sb.WriteRune(0x2078)
			case '9':
				sb.WriteRune(0x2079)
			case 'a':
				sb.WriteRune(0x1D43)
			case 'b':
				sb.WriteRune(0x1D47)
			case 'c':
				sb.WriteRune(0x1D48)
			case 'd':
				sb.WriteRune(0x1D49)
			case 'e':
				sb.WriteRune(0x1D4B)
			case 'f':
				sb.WriteRune(0x1D4C)
			case 'g':
				sb.WriteRune(0x1D4D)
			case 'h':
				sb.WriteRune(0x02B0)
			case 'i':
				sb.WriteRune(0x2071)
			case 'j':
				sb.WriteRune(0x02B2)
			case 'k':
				sb.WriteRune(0x1D4C)
			case 'l':
				sb.WriteRune(0x02E1)
			case 'm':
				sb.WriteRune(0x1D50)
			case 'n':
				sb.WriteRune(0x207F)
			case 'o':
				sb.WriteRune(0x1D52)
			case 'p':
				sb.WriteRune(0x1D56)
			case 'r':
				sb.WriteRune(0x02B3)
			case 's':
				sb.WriteRune(0x02E2)
			case 't':
				sb.WriteRune(0x1D57)
			case 'u':
				sb.WriteRune(0x1D58)
			case 'v':
				sb.WriteRune(0x1D5B)
			case 'w':
				sb.WriteRune(0x02B7)
			case 'x':
				sb.WriteRune(0x02E3)
			case 'y':
				sb.WriteRune(0x02B8)
			case 'z':
				sb.WriteRune(0x1D5C)
			default:
				sb.WriteRune(r)
			}
		case "parenthesized":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x249C)
			} else if r >= '1' && r <= '9' {
				sb.WriteRune(r - '1' + 0x2474)
			} else {
				sb.WriteRune(r)
			}
		case "bold-sans":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "regional-indicator":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1F1E6)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1F1E6)
			} else {
				sb.WriteRune(r)
			}
		case "bold-script-alt":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4B6)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D49C)
			} else {
				sb.WriteRune(r)
			}
		case "sans-serif-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "monospace-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D68A)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D670)
			} else {
				sb.WriteRune(r)
			}
		case "double-struck-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D552)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D538)
			} else {
				sb.WriteRune(r)
			}
		case "circled-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x24D0)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x24B6)
			} else {
				sb.WriteRune(r)
			}
		case "squared-bold":
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				if r >= 'a' && r <= 'z' {
					r -= 'a' - 'A'
				}
				sb.WriteRune(r - 'A' + 0x1F130)
			} else {
				sb.WriteRune(r)
			}
		case "small-caps-alt":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 'A')
			} else {
				sb.WriteRune(r)
			}
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
