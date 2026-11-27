package admin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRequestMetadataPresence(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		present     bool
		labels      map[string]string
		annotations map[string]string
	}{
		{"omitted", `{"display_name":"a"}`, false, map[string]string{}, map[string]string{}},
		{"empty object", `{"metadata":{}}`, true, map[string]string{}, map[string]string{}},
		{"labels only", `{"metadata":{"labels":{"tier":"gold"}}}`, true, map[string]string{"tier": "gold"}, map[string]string{}},
		{"annotations only", `{"metadata":{"annotations":{"note":"x"}}}`, true, map[string]string{}, map[string]string{"note": "x"}},
		{"both", `{"metadata":{"annotations":{"n":"é\n"},"labels":{"b":"2","a":"1"}}}`, true, map[string]string{"a": "1", "b": "2"}, map[string]string{"n": "é\n"}},
		{"non-object body", `null`, false, map[string]string{}, map[string]string{}},
		{"null labels are empty", `{"metadata":{"labels":null,"annotations":{"note":"x"}}}`, true, map[string]string{}, map[string]string{"note": "x"}},
		{"null annotations are empty", `{"metadata":{"labels":{"a":"1"},"annotations":null}}`, true, map[string]string{"a": "1"}, map[string]string{}},
		{"both maps null are empty", `{"metadata":{"labels":null,"annotations":null}}`, true, map[string]string{}, map[string]string{}},
		{"trailing bytes ignored", `{"metadata":{"labels":{"a":"1"}}} {"metadata":null} garbage`, true, map[string]string{"a": "1"}, map[string]string{}},
		{"other members keep lenient decoding", "{\"display_name\":\"\xff\",\"display_name\":\"b\",\"metadata\":{}}", true, map[string]string{}, map[string]string{}},
		{"nested metadata name is not top level", `{"other":{"metadata":null,"Metadata":1}}`, false, map[string]string{}, map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, present, err := decodeRequestMetadata([]byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, tc.present, present)
			assert.Equal(t, tc.labels, value.Labels())
			assert.Equal(t, tc.annotations, value.Annotations())
		})
	}
}

func TestDecodeRequestMetadataRejections(t *testing.T) {
	const sentinel = "zz-sentinel"
	cases := []struct {
		name string
		body string
		want string
	}{
		{"repeated member", `{"metadata":{},"metadata":{}}`, "request body must contain at most one metadata member spelled exactly metadata"},
		{"case variant", `{"Metadata":{}}`, "request body must contain at most one metadata member spelled exactly metadata"},
		{"case variant beside exact member", `{"metadata":{},"METADATA":{}}`, "request body must contain at most one metadata member spelled exactly metadata"},
		{"metadata null", `{"metadata":null}`, "metadata must not be null"},
		{"metadata string", `{"metadata":"x"}`, "metadata must be an object"},
		{"metadata array", `{"metadata":[]}`, "metadata must be an object"},
		{"unknown member", `{"metadata":{"zz-sentinel":{}}}`, "metadata contains an unknown or repeated member"},
		{"case variant member", `{"metadata":{"Labels":{}}}`, "metadata contains an unknown or repeated member"},
		{"invalid UTF-8 member name", "{\"metadata\":{\"labels\xff\":{}}}", "metadata contains an unknown or repeated member"},
		{"repeated member inside metadata", `{"metadata":{"labels":{},"labels":{}}}`, "metadata contains an unknown or repeated member"},
		{"labels array", `{"metadata":{"labels":["zz-sentinel"]}}`, "metadata.labels must be an object"},
		{"labels duplicate", `{"metadata":{"labels":{"zz-sentinel":"a","zz-sentinel":"b"}}}`, "metadata.labels contains a duplicate key"},
		{"labels invalid key", `{"metadata":{"labels":{"zz sentinel":"a"}}}`, "metadata.labels contains an invalid key"},
		{"labels invalid UTF-8 key", "{\"metadata\":{\"labels\":{\"zz-sentinel\xff\":\"a\"}}}", "metadata.labels contains an invalid key"},
		{"labels unpaired surrogate key", `{"metadata":{"labels":{"zz-sentinel\ud800":"a"}}}`, "metadata.labels contains an invalid key"},
		{"labels reserved key", `{"metadata":{"labels":{"agenticidentitybroker.dev/zz-sentinel":"a"}}}`, "metadata.labels contains a reserved key"},
		{"labels invalid value", `{"metadata":{"labels":{"k":"zz sentinel"}}}`, "metadata.labels contains an invalid value"},
		{"labels number value", `{"metadata":{"labels":{"k":1}}}`, "metadata.labels contains an invalid value"},
		{"labels null value", `{"metadata":{"labels":{"k":null}}}`, "metadata.labels contains an invalid value"},
		{"labels invalid UTF-8 value", "{\"metadata\":{\"labels\":{\"k\":\"zz-sentinel\xff\"}}}", "metadata.labels contains an invalid value"},
		{"labels too many", `{"metadata":{"labels":` + labelsJSON(65) + `}}`, "metadata.labels contains more than 64 entries"},
		{"annotations string", `{"metadata":{"annotations":"zz-sentinel"}}`, "metadata.annotations must be an object"},
		{"annotations duplicate", `{"metadata":{"annotations":{"zz-sentinel":"a","zz-sentinel":"a"}}}`, "metadata.annotations contains a duplicate key"},
		{"annotations invalid key", `{"metadata":{"annotations":{"zz sentinel":"a"}}}`, "metadata.annotations contains an invalid key"},
		{"annotations reserved key", `{"metadata":{"annotations":{"x.agenticidentitybroker.dev/zz-sentinel":"a"}}}`, "metadata.annotations contains a reserved key"},
		{"annotations null value", `{"metadata":{"annotations":{"zz-sentinel":null}}}`, "metadata.annotations contains an invalid value"},
		{"annotations NUL value", `{"metadata":{"annotations":{"k":"zz-sentinel\u0000"}}}`, "metadata.annotations contains an invalid value"},
		{"annotations unpaired surrogate value", `{"metadata":{"annotations":{"k":"zz-sentinel\udc00"}}}`, "metadata.annotations contains an invalid value"},
		{"annotations object value", `{"metadata":{"annotations":{"k":{"zz-sentinel":1}}}}`, "metadata.annotations contains an invalid value"},
		{"annotations too large", `{"metadata":{"annotations":{"k":"` + strings.Repeat("a", 65536) + `"}}}`, "metadata.annotations exceeds 65536 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, _, err := decodeRequestMetadata([]byte(tc.body))
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.NotContains(t, err.Error(), sentinel)
			assert.NotContains(t, err.Error(), "jsontext")
			assert.NotContains(t, err.Error(), "duplicate object member")
			assert.True(t, value.IsEmpty())
		})
	}
}

func TestDecodeRequestMetadataCheckOrder(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"request member before shape", `{"metadata":null,"Metadata":{}}`, "request body must contain at most one metadata member spelled exactly metadata"},
		{"unknown member after a null map", `{"metadata":{"labels":null,"other":{}}}`, "metadata contains an unknown or repeated member"},
		{"decoding errors win over entry rules", `{"metadata":{"labels":{"bad key":"v"},"annotations":{"k":1}}}`, "metadata.annotations contains an invalid value"},
		{"decoding errors in document order", `{"metadata":{"annotations":{"k":1},"labels":{"k":1}}}`, "metadata.annotations contains an invalid value"},
		{"labels before annotations in any document order", `{"metadata":{"annotations":{"bad key":"v"},"labels":{"bad key":"v"}}}`, "metadata.labels contains an invalid key"},
		{"label count before annotations", `{"metadata":{"annotations":{"bad key":"v"},"labels":` + labelsJSON(65) + `}}`, "metadata.labels contains more than 64 entries"},
		{"entries in ascending key order", `{"metadata":{"labels":{"k":"bad value","bad key":"v"}}}`, "metadata.labels contains an invalid key"},
		{"duplicate before key grammar", `{"metadata":{"labels":{"bad key":"v","bad key":"v"}}}`, "metadata.labels contains a duplicate key"},
		{"duplicate of valid key", `{"metadata":{"labels":{"k":"v","k":"bad value"}}}`, "metadata.labels contains a duplicate key"},
		{"non-string value before key grammar", `{"metadata":{"labels":{"bad key":1}}}`, "metadata.labels contains an invalid value"},
		{"key grammar before value grammar", `{"metadata":{"labels":{"bad key":"bad value"}}}`, "metadata.labels contains an invalid key"},
		{"reserved before value grammar", `{"metadata":{"annotations":{"agenticidentitybroker.dev/x":"\u0000"}}}`, "metadata.annotations contains a reserved key"},
		{"entry before count", `{"metadata":{"labels":` + strings.TrimSuffix(labelsJSON(65), "}") + `,"bad key":"v"}}}`, "metadata.labels contains an invalid key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := decodeRequestMetadata([]byte(tc.body))
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
		})
	}
}

func TestDecodeRequestMetadataAcceptsLimits(t *testing.T) {
	body := `{"metadata":{"labels":` + labelsJSON(64) + `,"annotations":{"k":"` + strings.Repeat("a", 65535) + `"}}}`
	value, present, err := decodeRequestMetadata([]byte(body))
	require.NoError(t, err)
	assert.True(t, present)
	assert.Len(t, value.Labels(), 64)
}

func TestObjectMetadataResponse(t *testing.T) {
	t.Run("empty metadata is omitted", func(t *testing.T) {
		assert.Nil(t, objectMetadataResponse(model.ObjectMetadata{}))
		encoded, err := json.Marshal(struct {
			Metadata *ObjectMetadataResponse `json:"metadata,omitempty"`
		}{objectMetadataResponse(model.ObjectMetadata{})})
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(encoded))
	})
	t.Run("empty map is omitted", func(t *testing.T) {
		value, err := model.NewObjectMetadata(map[string]string{"a": "1"}, nil)
		require.NoError(t, err)
		encoded, err := json.Marshal(objectMetadataResponse(value))
		require.NoError(t, err)
		assert.Equal(t, `{"labels":{"a":"1"}}`, string(encoded))
	})
	t.Run("keys are in byte order", func(t *testing.T) {
		value, err := model.NewObjectMetadata(map[string]string{"b": "2", "B": "3", "a": "1"}, map[string]string{"z": "", "y": "x"})
		require.NoError(t, err)
		encoded, err := json.Marshal(objectMetadataResponse(value))
		require.NoError(t, err)
		assert.Equal(t, `{"labels":{"B":"3","a":"1","b":"2"},"annotations":{"y":"x","z":""}}`, string(encoded))
	})
}

func labelsJSON(count int) string {
	entries := make([]string, count)
	for i := range count {
		entries[i] = `"label-` + strings.Repeat("x", i/26) + string(rune('a'+i%26)) + `":"v"`
	}
	return "{" + strings.Join(entries, ",") + "}"
}
