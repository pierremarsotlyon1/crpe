// Package grading implements the numeric rules shared by practice, exams and tutor feedback.
package grading

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var decimal = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]{1,3})?$`)
var fraction = regexp.MustCompile(`^([+-]?[0-9]+)/([0-9]+)$`)

// Number parses exact decimal values and fractions. Bound input length and
// exponent size before allocating rationals; parse fraction integers as base ten.
func Number(input string) (*big.Rat, bool) {
	v := strings.ReplaceAll(strings.TrimSpace(input), ",", ".")
	if len(v) > 256 {
		return nil, false
	}
	if parts := fraction.FindStringSubmatch(v); parts != nil {
		n, ok := new(big.Int).SetString(parts[1], 10)
		if !ok {
			return nil, false
		}
		d, ok := new(big.Int).SetString(parts[2], 10)
		if !ok || d.Sign() == 0 {
			return nil, false
		}
		return new(big.Rat).SetFrac(n, d), true
	}
	if !decimal.MatchString(v) {
		return nil, false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsInf(n, 0) {
		return nil, false
	}
	if !strings.ContainsAny(v, ".eE") {
		v += ".0"
	}
	return new(big.Rat).SetString(v)
}

// Numeric returns a verdict only when both values and their tolerance are valid.
func Numeric(reference, answer string, tolerance float64) (correct, known bool) {
	if tolerance < 0 || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) {
		return false, false
	}
	want, ok := Number(reference)
	if !ok {
		return false, false
	}
	got, ok := Number(answer)
	if !ok {
		return false, false
	}
	allowed, ok := new(big.Rat).SetString(strconv.FormatFloat(tolerance, 'g', -1, 64))
	if !ok {
		return false, false
	}
	delta := new(big.Rat).Sub(want, got)
	return delta.Abs(delta).Cmp(allowed) <= 0, true
}
