package domain

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var usernameFold = cases.Fold()

func NormalizeUsername(value string) string {
	return usernameFold.String(norm.NFKC.String(strings.TrimSpace(value)))
}
