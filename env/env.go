package env

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	charComment       = '#'
	prefixSingleQuote = '\''
	prefixDoubleQuote = '"'

	exportPrefix = "export"
)

func Load(filenames ...string) error {
	for _, filename := range filenamesOrDefault(filenames) {
		if err := loadFile(filename, false); err != nil {
			return err
		}
	}
	return nil
}

func String(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Int(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func Bool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func Duration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func filenamesOrDefault(filenames []string) []string {
	if len(filenames) == 0 {
		return []string{".env"}
	}
	return filenames
}

func loadFile(filename string, overload bool) error {
	envMap, err := readFile(filename)
	if err != nil {
		return err
	}

	currentEnv := map[string]bool{}
	for _, rawEnvLine := range os.Environ() {
		key := strings.Split(rawEnvLine, "=")[0]
		currentEnv[key] = true
	}

	for key, value := range envMap {
		if !currentEnv[key] || overload {
			_ = os.Setenv(key, value)
		}
	}

	return nil
}

func readFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return Parse(file)
}

func Parse(r io.Reader) (map[string]string, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	return UnmarshalBytes(buf.Bytes())
}

func UnmarshalBytes(src []byte) (map[string]string, error) {
	out := make(map[string]string)
	err := parseBytes(src, out)
	return out, err
}

func parseBytes(src []byte, out map[string]string) error {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	cutset := src
	for {
		cutset = getStatementStart(cutset)
		if cutset == nil {
			break
		}

		key, left, err := locateKeyName(cutset)
		if err != nil {
			return err
		}

		value, left, err := extractVarValue(left, out)
		if err != nil {
			return err
		}

		out[key] = value
		cutset = left
	}

	return nil
}

func getStatementStart(src []byte) []byte {
	pos := indexOfNonSpaceChar(src)
	if pos == -1 {
		return nil
	}

	src = src[pos:]
	if src[0] != charComment {
		return src
	}

	pos = bytes.IndexFunc(src, isCharFunc('\n'))
	if pos == -1 {
		return nil
	}

	return getStatementStart(src[pos:])
}

func locateKeyName(src []byte) (key string, cutset []byte, err error) {
	src = bytes.TrimLeftFunc(src, isSpace)
	if bytes.HasPrefix(src, []byte(exportPrefix)) {
		trimmed := bytes.TrimPrefix(src, []byte(exportPrefix))
		if bytes.IndexFunc(trimmed, isSpace) == 0 {
			src = bytes.TrimLeftFunc(trimmed, isSpace)
		}
	}

	offset := 0
loop:
	for i, char := range src {
		rchar := rune(char)
		if isSpace(rchar) {
			continue
		}

		switch char {
		case '=', ':':
			key = string(src[0:i])
			offset = i + 1
			break loop
		case '_':
		default:
			if unicode.IsLetter(rchar) || unicode.IsNumber(rchar) || rchar == '.' {
				continue
			}

			return "", nil, fmt.Errorf(`unexpected character %q in variable name near %q`, string(char), string(src))
		}
	}

	if len(src) == 0 {
		return "", nil, errors.New("zero length string")
	}

	key = strings.TrimRightFunc(key, unicode.IsSpace)
	cutset = bytes.TrimLeftFunc(src[offset:], isSpace)
	return key, cutset, nil
}

func extractVarValue(src []byte, vars map[string]string) (value string, rest []byte, err error) {
	quote, hasPrefix := hasQuotePrefix(src)
	if !hasPrefix {
		endOfLine := bytes.IndexFunc(src, isLineEnd)
		if endOfLine == -1 {
			endOfLine = len(src)
			if endOfLine == 0 {
				return "", nil, nil
			}
		}

		line := []rune(string(src[0:endOfLine]))
		endOfVar := len(line)
		if endOfVar == 0 {
			return "", src[endOfLine:], nil
		}

		for i := endOfVar - 1; i >= 0; i-- {
			if line[i] == charComment && i > 0 && isSpace(line[i-1]) {
				endOfVar = i
				break
			}
		}

		trimmed := strings.TrimFunc(string(line[0:endOfVar]), isSpace)
		return expandVariables(trimmed, vars), src[endOfLine:], nil
	}

	for i := 1; i < len(src); i++ {
		if char := src[i]; char != quote {
			continue
		}
		if prevChar := src[i-1]; prevChar == '\\' {
			continue
		}

		trimFunc := isCharFunc(rune(quote))
		value = string(bytes.TrimLeftFunc(bytes.TrimRightFunc(src[0:i], trimFunc), trimFunc))
		if quote == prefixDoubleQuote {
			value = expandVariables(expandEscapes(value), vars)
		}

		return value, src[i+1:], nil
	}

	valEndIndex := bytes.IndexFunc(src, isCharFunc('\n'))
	if valEndIndex == -1 {
		valEndIndex = len(src)
	}

	return "", nil, fmt.Errorf("unterminated quoted value %s", src[:valEndIndex])
}

func expandEscapes(str string) string {
	out := escapeRegex.ReplaceAllStringFunc(str, func(match string) string {
		c := strings.TrimPrefix(match, `\`)
		switch c {
		case "n":
			return "\n"
		case "r":
			return "\r"
		default:
			return match
		}
	})
	return unescapeCharsRegex.ReplaceAllString(out, "$1")
}

func indexOfNonSpaceChar(src []byte) int {
	return bytes.IndexFunc(src, func(r rune) bool {
		return !unicode.IsSpace(r)
	})
}

var (
	escapeRegex        = regexp.MustCompile(`\\.`)
	expandVarRegex     = regexp.MustCompile(`(\\)?(\$)(\()?\{?([A-Z0-9_]+)?\}?`)
	unescapeCharsRegex = regexp.MustCompile(`\\([^$])`)
)

func expandVariables(v string, m map[string]string) string {
	return expandVarRegex.ReplaceAllStringFunc(v, func(s string) string {
		submatch := expandVarRegex.FindStringSubmatch(s)

		if submatch == nil {
			return s
		}
		if submatch[1] == "\\" || submatch[2] == "(" {
			return submatch[0][1:]
		} else if submatch[4] != "" {
			return m[submatch[4]]
		}
		return s
	})
}
