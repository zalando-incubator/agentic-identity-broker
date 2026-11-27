package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewObjectMetadataLabelKeys(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want error
	}{
		{"plain name", "deployment", nil},
		{"prefixed name", "team.example.com/owner", nil},
		{"name with inner punctuation", "a-b_c.d", nil},
		{"63 name characters", strings.Repeat("a", 63), nil},
		{"64 name characters", strings.Repeat("a", 64), ErrMetadataLabelsInvalidKey},
		{"253 prefix characters", strings.Repeat("a", 253) + "/x", nil},
		{"254 prefix characters", strings.Repeat("a", 254) + "/x", ErrMetadataLabelsInvalidKey},
		{"two slashes", "a.example/b/c", ErrMetadataLabelsInvalidKey},
		{"empty prefix", "/name", ErrMetadataLabelsInvalidKey},
		{"empty name", "example.com/", ErrMetadataLabelsInvalidKey},
		{"empty key", "", ErrMetadataLabelsInvalidKey},
		{"uppercase prefix", "Example.com/name", ErrMetadataLabelsInvalidKey},
		{"leading whitespace", " name", ErrMetadataLabelsInvalidKey},
		{"trailing whitespace", "name ", ErrMetadataLabelsInvalidKey},
		{"tab", "na\tme", ErrMetadataLabelsInvalidKey},
		{"non-ASCII look-alike", "nаme", ErrMetadataLabelsInvalidKey},
		{"leading punctuation", "-name", ErrMetadataLabelsInvalidKey},
		{"NUL", "na\x00me", ErrMetadataLabelsInvalidKey},
		{"reserved domain", "agenticidentitybroker.dev/x", ErrMetadataLabelsReservedKey},
		{"reserved subdomain", "policy.agenticidentitybroker.dev/x", ErrMetadataLabelsReservedKey},
		{"look-alike domain", "example.agenticidentitybroker.dev.evil.com/x", nil},
		{"look-alike suffix without dot", "xagenticidentitybroker.dev/x", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewObjectMetadata(map[string]string{tc.key: "v"}, nil)
			assertMetadataError(t, err, tc.want, tc.key)
		})
	}
}

func TestNewObjectMetadataAnnotationKeys(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want error
	}{
		{"prefixed name", "gitops.example.com/revision", nil},
		{"64 name characters", strings.Repeat("a", 64), ErrMetadataAnnotationsInvalidKey},
		{"reserved domain", "agenticidentitybroker.dev/x", ErrMetadataAnnotationsReservedKey},
		{"reserved subdomain", "a.agenticidentitybroker.dev/x", ErrMetadataAnnotationsReservedKey},
		{"look-alike domain", "example.agenticidentitybroker.dev.evil.com/x", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewObjectMetadata(nil, map[string]string{tc.key: "v"})
			assertMetadataError(t, err, tc.want, tc.key)
		})
	}
}

func TestNewObjectMetadataValues(t *testing.T) {
	t.Run("labels", func(t *testing.T) {
		cases := []struct {
			name  string
			value string
			want  error
		}{
			{"empty", "", nil},
			{"63 characters", strings.Repeat("v", 63), nil},
			{"64 characters", strings.Repeat("v", 64), ErrMetadataLabelsInvalidValue},
			{"slash", "a/b", ErrMetadataLabelsInvalidValue},
			{"whitespace", "a b", ErrMetadataLabelsInvalidValue},
			{"NUL", "a\x00b", ErrMetadataLabelsInvalidValue},
			{"multi-byte", "café", ErrMetadataLabelsInvalidValue},
			{"invalid UTF-8", "a\xffb", ErrMetadataLabelsInvalidValue},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := NewObjectMetadata(map[string]string{"key": tc.value}, nil)
				assertMetadataError(t, err, tc.want, tc.value)
			})
		}
	})
	t.Run("annotations", func(t *testing.T) {
		cases := []struct {
			name  string
			value string
			want  error
		}{
			{"empty", "", nil},
			{"free text", "Owned by the platform team. 世界 \n\t", nil},
			{"NUL", "a\x00b", ErrMetadataAnnotationsInvalidValue},
			{"invalid UTF-8", "a\xffb", ErrMetadataAnnotationsInvalidValue},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := NewObjectMetadata(nil, map[string]string{"key": tc.value})
				assertMetadataError(t, err, tc.want, tc.value)
			})
		}
	})
}

func TestNewObjectMetadataLimits(t *testing.T) {
	t.Run("64 labels accepted", func(t *testing.T) {
		_, err := NewObjectMetadata(labelsOfCount(64), nil)
		require.NoError(t, err)
	})
	t.Run("65 labels rejected", func(t *testing.T) {
		_, err := NewObjectMetadata(labelsOfCount(65), nil)
		assert.ErrorIs(t, err, ErrMetadataLabelsTooMany)
	})
	t.Run("65536 annotation bytes accepted", func(t *testing.T) {
		_, err := NewObjectMetadata(nil, annotationsOfSize(65536))
		require.NoError(t, err)
	})
	t.Run("65537 annotation bytes rejected", func(t *testing.T) {
		_, err := NewObjectMetadata(nil, annotationsOfSize(65537))
		assert.ErrorIs(t, err, ErrMetadataAnnotationsTooLarge)
	})
	t.Run("multi-byte characters count as UTF-8 bytes", func(t *testing.T) {
		value := strings.Repeat("é", 32766) // 65,532 bytes
		_, err := NewObjectMetadata(nil, map[string]string{"abcd": value})
		require.NoError(t, err)
		_, err = NewObjectMetadata(nil, map[string]string{"abcde": value})
		assert.ErrorIs(t, err, ErrMetadataAnnotationsTooLarge)
	})
	t.Run("label errors win over annotation errors", func(t *testing.T) {
		_, err := NewObjectMetadata(map[string]string{"bad key": "v"}, map[string]string{"bad key": "v"})
		assert.ErrorIs(t, err, ErrMetadataLabelsInvalidKey)
	})
}

func TestObjectMetadataValue(t *testing.T) {
	t.Run("zero value is valid and empty", func(t *testing.T) {
		var zero ObjectMetadata
		assert.True(t, zero.IsEmpty())
		assert.NoError(t, zero.Validate())
		assert.Empty(t, zero.Labels())
		assert.NotNil(t, zero.Labels())
		assert.Empty(t, zero.Annotations())
		assert.NotNil(t, zero.Annotations())
	})
	t.Run("nil and empty inputs are empty", func(t *testing.T) {
		value, err := NewObjectMetadata(map[string]string{}, nil)
		require.NoError(t, err)
		assert.True(t, value.IsEmpty())
		assert.True(t, value.Equal(ObjectMetadata{}))
	})
	t.Run("input and output are copied", func(t *testing.T) {
		labels := map[string]string{"tier": "gold"}
		annotations := map[string]string{"note": "x"}
		value, err := NewObjectMetadata(labels, annotations)
		require.NoError(t, err)
		labels["tier"] = "silver"
		annotations["note"] = "y"
		out := value.Labels()
		out["tier"] = "bronze"
		value.Annotations()["note"] = "z"
		assert.Equal(t, map[string]string{"tier": "gold"}, value.Labels())
		assert.Equal(t, map[string]string{"note": "x"}, value.Annotations())
	})
	t.Run("case-distinct keys stay distinct", func(t *testing.T) {
		value, err := NewObjectMetadata(map[string]string{"exposure": "internal", "Exposure": "public"}, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"exposure": "internal", "Exposure": "public"}, value.Labels())
	})
	t.Run("equality ignores order and detects changes", func(t *testing.T) {
		a, err := NewObjectMetadata(map[string]string{"a": "1", "b": "2"}, map[string]string{"x": "y"})
		require.NoError(t, err)
		b, err := NewObjectMetadata(map[string]string{"b": "2", "a": "1"}, map[string]string{"x": "y"})
		require.NoError(t, err)
		changedLabel, err := NewObjectMetadata(map[string]string{"a": "1", "b": "3"}, map[string]string{"x": "y"})
		require.NoError(t, err)
		changedAnnotation, err := NewObjectMetadata(map[string]string{"a": "1", "b": "2"}, map[string]string{"x": "z"})
		require.NoError(t, err)
		movedMap, err := NewObjectMetadata(nil, map[string]string{"a": "1", "b": "2", "x": "y"})
		require.NoError(t, err)
		assert.True(t, a.Equal(b))
		assert.False(t, a.Equal(changedLabel))
		assert.False(t, a.Equal(changedAnnotation))
		assert.False(t, a.Equal(movedMap))
		assert.False(t, a.IsEmpty())
	})
}

func TestRestoreObjectMetadata(t *testing.T) {
	t.Run("copies stored maps without applying a rule", func(t *testing.T) {
		labels := map[string]string{"agenticidentitybroker.dev/x": "v", "bad key": "bad value"}
		annotations := map[string]string{"a.agenticidentitybroker.dev/y": "v"}
		value := RestoreObjectMetadata(labels, annotations)
		labels["other"] = "x"
		assert.Equal(t, map[string]string{"agenticidentitybroker.dev/x": "v", "bad key": "bad value"}, value.Labels())
		assert.Equal(t, map[string]string{"a.agenticidentitybroker.dev/y": "v"}, value.Annotations())
	})
	t.Run("nil means empty", func(t *testing.T) {
		assert.True(t, RestoreObjectMetadata(nil, nil).IsEmpty())
	})
}

func TestObjectMetadataValidate(t *testing.T) {
	cases := []struct {
		name  string
		value ObjectMetadata
		want  error
	}{
		{"reserved label key accepted", RestoreObjectMetadata(map[string]string{"agenticidentitybroker.dev/x": "v"}, nil), nil},
		{"reserved annotation key accepted", RestoreObjectMetadata(nil, map[string]string{"a.agenticidentitybroker.dev/x": "v"}), nil},
		{"label grammar", RestoreObjectMetadata(map[string]string{"bad key": "v"}, nil), ErrMetadataLabelsInvalidKey},
		{"label value", RestoreObjectMetadata(map[string]string{"k": "bad value"}, nil), ErrMetadataLabelsInvalidValue},
		{"annotation NUL", RestoreObjectMetadata(nil, map[string]string{"k": "\x00"}), ErrMetadataAnnotationsInvalidValue},
		{"label count", RestoreObjectMetadata(labelsOfCount(65), nil), ErrMetadataLabelsTooMany},
		{"annotation size", RestoreObjectMetadata(nil, annotationsOfSize(65537)), ErrMetadataAnnotationsTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertMetadataError(t, tc.value.Validate(), tc.want, "")
		})
	}
}

func TestCheckMetadataEntries(t *testing.T) {
	assert.NoError(t, CheckLabelEntry("tier", "gold"))
	assert.ErrorIs(t, CheckLabelEntry("bad key", "bad value"), ErrMetadataLabelsInvalidKey)
	assert.ErrorIs(t, CheckLabelEntry("agenticidentitybroker.dev/x", "bad value"), ErrMetadataLabelsReservedKey)
	assert.ErrorIs(t, CheckLabelEntry("tier", "bad value"), ErrMetadataLabelsInvalidValue)
	assert.NoError(t, CheckAnnotationEntry("note", "free text"))
	assert.ErrorIs(t, CheckAnnotationEntry("bad key", "\x00"), ErrMetadataAnnotationsInvalidKey)
	assert.ErrorIs(t, CheckAnnotationEntry("agenticidentitybroker.dev/x", "\x00"), ErrMetadataAnnotationsReservedKey)
	assert.ErrorIs(t, CheckAnnotationEntry("note", "\x00"), ErrMetadataAnnotationsInvalidValue)
}

func assertMetadataError(t *testing.T, err, want error, submitted string) {
	t.Helper()
	if want == nil {
		assert.NoError(t, err)
		return
	}
	require.ErrorIs(t, err, want)
	assert.Equal(t, want.Error(), err.Error())
	if strings.TrimSpace(submitted) != "" {
		assert.NotContains(t, err.Error(), submitted)
	}
}

func labelsOfCount(count int) map[string]string {
	labels := make(map[string]string, count)
	for i := range count {
		labels["label-"+strings.Repeat("x", i%10)+string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	return labels
}

func annotationsOfSize(size int) map[string]string {
	const key = "note"
	return map[string]string{key: strings.Repeat("a", size-len(key))}
}
