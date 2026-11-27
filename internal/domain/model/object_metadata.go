package model

import (
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxLabels              = 64
	maxAnnotationBytes     = 65536
	maxMetadataKeyPrefix   = 253
	maxMetadataKeyName     = 63
	maxLabelValue          = 63
	reservedMetadataDomain = "agenticidentitybroker.dev"
)

// Messages are fixed so that no error can carry a submitted key or value.
var (
	ErrMetadataLabelsInvalidKey        = errors.New("metadata.labels contains an invalid key")
	ErrMetadataLabelsReservedKey       = errors.New("metadata.labels contains a reserved key")
	ErrMetadataLabelsInvalidValue      = errors.New("metadata.labels contains an invalid value")
	ErrMetadataLabelsTooMany           = errors.New("metadata.labels contains more than 64 entries")
	ErrMetadataAnnotationsInvalidKey   = errors.New("metadata.annotations contains an invalid key")
	ErrMetadataAnnotationsReservedKey  = errors.New("metadata.annotations contains a reserved key")
	ErrMetadataAnnotationsInvalidValue = errors.New("metadata.annotations contains an invalid value")
	ErrMetadataAnnotationsTooLarge     = errors.New("metadata.annotations exceeds 65536 bytes")
)

var (
	metadataKeyPrefixPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	metadataKeyNamePattern   = regexp.MustCompile(`^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`)
)

// ObjectMetadata is the immutable label and annotation pair carried by Agents,
// Services, and Permission Sets. The zero value is valid and empty.
type ObjectMetadata struct {
	labels      map[string]string
	annotations map[string]string
}

// NewObjectMetadata builds a value from an Admin write and applies every
// write rule, including the reserved prefix. A nil map means empty.
func NewObjectMetadata(labels, annotations map[string]string) (ObjectMetadata, error) {
	value := RestoreObjectMetadata(labels, annotations)
	if err := value.check(true); err != nil {
		return ObjectMetadata{}, err
	}
	return value, nil
}

// RestoreObjectMetadata rebuilds a stored value without applying any rule, so
// stored metadata can never fail a read.
func RestoreObjectMetadata(labels, annotations map[string]string) ObjectMetadata {
	return ObjectMetadata{labels: copyNonEmpty(labels), annotations: copyNonEmpty(annotations)}
}

// Validate re-checks every write rule except the reserved prefix, which only
// the Admin write constructor enforces.
func (m ObjectMetadata) Validate() error {
	return m.check(false)
}

func (m ObjectMetadata) Labels() map[string]string {
	return copyAlways(m.labels)
}

func (m ObjectMetadata) Annotations() map[string]string {
	return copyAlways(m.annotations)
}

func (m ObjectMetadata) Equal(other ObjectMetadata) bool {
	return maps.Equal(m.labels, other.labels) && maps.Equal(m.annotations, other.annotations)
}

func (m ObjectMetadata) IsEmpty() bool {
	return len(m.labels) == 0 && len(m.annotations) == 0
}

// CheckLabelEntry applies the per-entry label rules: key grammar, reserved
// prefix, then value grammar.
func CheckLabelEntry(key, value string) error {
	return checkLabelEntry(key, value, true)
}

// CheckAnnotationEntry applies the per-entry annotation rules: key grammar,
// reserved prefix, then value encoding.
func CheckAnnotationEntry(key, value string) error {
	return checkAnnotationEntry(key, value, true)
}

func checkLabelEntry(key, value string, reserved bool) error {
	switch keyStatus(key) {
	case keyInvalid:
		return ErrMetadataLabelsInvalidKey
	case keyReserved:
		if reserved {
			return ErrMetadataLabelsReservedKey
		}
	}
	if value != "" && !validNameSegment(value, maxLabelValue) {
		return ErrMetadataLabelsInvalidValue
	}
	return nil
}

func checkAnnotationEntry(key, value string, reserved bool) error {
	switch keyStatus(asciiLower(key)) {
	case keyInvalid:
		return ErrMetadataAnnotationsInvalidKey
	case keyReserved:
		if reserved {
			return ErrMetadataAnnotationsReservedKey
		}
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return ErrMetadataAnnotationsInvalidValue
	}
	return nil
}

func (m ObjectMetadata) check(reserved bool) error {
	for _, key := range slices.Sorted(maps.Keys(m.labels)) {
		if err := checkLabelEntry(key, m.labels[key], reserved); err != nil {
			return err
		}
	}
	if len(m.labels) > maxLabels {
		return ErrMetadataLabelsTooMany
	}
	size := 0
	for _, key := range slices.Sorted(maps.Keys(m.annotations)) {
		value := m.annotations[key]
		if err := checkAnnotationEntry(key, value, reserved); err != nil {
			return err
		}
		size += len(key) + len(value)
	}
	if size > maxAnnotationBytes {
		return ErrMetadataAnnotationsTooLarge
	}
	return nil
}

type metadataKeyStatus int

const (
	keyValid metadataKeyStatus = iota
	keyInvalid
	keyReserved
)

func keyStatus(key string) metadataKeyStatus {
	prefix, name, hasPrefix := strings.Cut(key, "/")
	if !hasPrefix {
		name = key
	}
	if strings.Contains(name, "/") || !validNameSegment(name, maxMetadataKeyName) {
		return keyInvalid
	}
	if !hasPrefix {
		return keyValid
	}
	if len(prefix) == 0 || len(prefix) > maxMetadataKeyPrefix || !metadataKeyPrefixPattern.MatchString(prefix) {
		return keyInvalid
	}
	if prefix == reservedMetadataDomain || strings.HasSuffix(prefix, "."+reservedMetadataDomain) {
		return keyReserved
	}
	return keyValid
}

// asciiLower folds only A-Z. Annotation keys ignore ASCII case in their
// grammar, as Kubernetes does, while strings.ToLower would also map non-ASCII
// look-alikes such as the Kelvin sign onto ASCII letters.
func asciiLower(key string) string {
	return strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, key)
}

func validNameSegment(segment string, maxLength int) bool {
	return len(segment) > 0 && len(segment) <= maxLength && metadataKeyNamePattern.MatchString(segment)
}

func copyNonEmpty(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	return maps.Clone(source)
}

func copyAlways(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	maps.Copy(result, source)
	return result
}
