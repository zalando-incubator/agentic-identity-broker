package admin

import (
	"errors"
	"time"
)

func parseISO8601Duration(input string) (time.Duration, error) {
	invalid := errors.New("invalid ISO 8601 duration")
	if len(input) < 3 || input[0] != 'P' {
		return 0, invalid
	}

	const maxDuration = uint64(1<<63 - 1)
	var total uint64
	lastUnit := 0
	inTime := false
	for i := 1; i < len(input); {
		if input[i] == 'T' {
			if inTime || i+1 == len(input) {
				return 0, invalid
			}
			inTime = true
			i++
			continue
		}

		if input[i] < '0' || input[i] > '9' {
			return 0, invalid
		}
		var whole uint64
		for i < len(input) && input[i] >= '0' && input[i] <= '9' {
			digit := uint64(input[i] - '0')
			if whole > (maxDuration-digit)/10 {
				return 0, invalid
			}
			whole = whole*10 + digit
			i++
		}

		var fraction uint64
		fractionDigits := 0
		if i < len(input) && input[i] == '.' {
			i++
			for i < len(input) && input[i] >= '0' && input[i] <= '9' {
				if fractionDigits < 9 {
					fraction = fraction*10 + uint64(input[i]-'0')
				} else if input[i] != '0' {
					return 0, invalid // A sub-nanosecond value cannot be represented exactly.
				}
				fractionDigits++
				i++
			}
			if fractionDigits == 0 {
				return 0, invalid
			}
			for padding := fractionDigits; padding < 9; padding++ {
				fraction *= 10
			}
		}
		if i == len(input) {
			return 0, invalid
		}

		var unit uint64
		var rank int
		switch input[i] {
		case 'D':
			unit, rank = uint64(24*time.Hour), 1
			if inTime {
				return 0, invalid
			}
		case 'H':
			unit, rank = uint64(time.Hour), 2
		case 'M':
			unit, rank = uint64(time.Minute), 3
		case 'S':
			unit, rank = uint64(time.Second), 4
		default:
			return 0, invalid
		}
		if rank <= lastUnit || (rank > 1 && !inTime) || (fractionDigits > 0 && rank != 4) {
			return 0, invalid
		}
		lastUnit = rank
		if whole > (maxDuration-total)/unit {
			return 0, invalid
		}
		total += whole * unit
		if fraction > maxDuration-total {
			return 0, invalid
		}
		total += fraction
		i++
	}
	if total == 0 {
		return 0, invalid
	}
	return time.Duration(total), nil
}
