package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseISO8601DurationValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "minutes", input: "PT5M", want: 5 * time.Minute},
		{name: "days", input: "P1D", want: 24 * time.Hour},
		{name: "fractional seconds", input: "PT1.5S", want: time.Second + 500*time.Millisecond},
		{name: "combined units", input: "P2DT3H4M5.125S", want: 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second + 125*time.Millisecond},
		{name: "zero component in positive duration", input: "PT0H1M", want: time.Minute},
		{name: "nanosecond precision", input: "PT0.000000001S", want: time.Nanosecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseISO8601Duration(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseISO8601DurationInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "zero days", input: "P0D"},
		{name: "zero seconds", input: "PT0S"},
		{name: "all zero components", input: "P0DT0H0M0S"},
		{name: "negative duration", input: "-PT5M"},
		{name: "negative component", input: "PT-5M"},
		{name: "overflow in days", input: "P106752D"},
		{name: "overflow in seconds", input: "PT9223372037S"},
		{name: "date designator alone", input: "P"},
		{name: "time designator alone", input: "PT"},
		{name: "time designator without time component", input: "P1DT"},
		{name: "years", input: "P1Y"},
		{name: "weeks", input: "P1W"},
		{name: "calendar months", input: "P1M"},
		{name: "Go duration", input: "5m"},
		{name: "decimal without whole seconds", input: "PT.5S"},
		{name: "decimal without fractional seconds", input: "PT1.S"},
		{name: "fractional minutes", input: "PT1.5M"},
		{name: "missing unit", input: "PT5"},
		{name: "extra trailing text", input: "PT5Mfoo"},
		{name: "leading whitespace", input: " PT5M"},
		{name: "hours after minutes", input: "PT5M1H"},
		{name: "minutes after seconds", input: "PT5S1M"},
		{name: "day after time", input: "PT1H1D"},
		{name: "repeated days", input: "P1D2D"},
		{name: "repeated minutes", input: "PT1M2M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseISO8601Duration(tt.input)
			require.Error(t, err)
		})
	}
}
