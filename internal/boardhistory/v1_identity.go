package boardhistory

import (
	"errors"
	"strings"
)

// These checks own the retained V1 grammar. They do not use product parsers
// whose admission rules may change while retained history remains readable.
func validateBoardIDV1(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, " \t\r\n") {
		return errors.New("board identity required without whitespace")
	}
	return nil
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || isASCIILower(value)
}

func isASCIILower(value byte) bool { return value >= 'a' && value <= 'z' }
func isASCIIDigit(value byte) bool { return value >= '0' && value <= '9' }
