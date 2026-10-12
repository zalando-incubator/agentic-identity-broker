package configutil

import (
	"strings"

	"github.com/spf13/viper"
)

// Delimiter is reserved for structural configuration paths; dots remain literal.
const Delimiter = "::"

func NewViper() *viper.Viper {
	return viper.NewWithOptions(viper.KeyDelimiter(Delimiter))
}

func Key(parts ...string) string {
	return strings.Join(parts, Delimiter)
}
