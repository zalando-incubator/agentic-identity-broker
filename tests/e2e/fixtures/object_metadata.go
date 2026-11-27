package fixtures

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Sentinel metadata is distinctive enough that its presence in any response,
// log record, span, or metric attribute can only come from a leak.
const (
	MetadataSentinelLabelKey        = "sentinel.example.com/zz-label-7f3a9c"
	MetadataSentinelLabelValue      = "zz-label-value-7f3a9c"
	MetadataSentinelAnnotationKey   = "sentinel.example.com/zz-annotation-7f3a9c"
	MetadataSentinelAnnotationValue = "zz-annotation-value-7f3a9c 東京"
)

// MetadataSentinels lists every sentinel key and value.
func MetadataSentinels() []string {
	return []string{
		MetadataSentinelLabelKey, MetadataSentinelLabelValue,
		MetadataSentinelAnnotationKey, "zz-annotation-value-7f3a9c",
	}
}

// SentinelLabels returns ordinary labels plus the sentinel label.
func SentinelLabels() map[string]string {
	return map[string]string{
		"deployment":             "local",
		"team.example.com/owner": "platform",
		MetadataSentinelLabelKey: MetadataSentinelLabelValue,
	}
}

// SentinelAnnotations returns ordinary annotations plus the sentinel annotation.
func SentinelAnnotations() map[string]string {
	return map[string]string{
		"gitops.example.com/revision": "4f2c9e1",
		MetadataSentinelAnnotationKey: MetadataSentinelAnnotationValue,
	}
}

// SentinelMetadataJSON returns a metadata member value with both sentinel maps.
func SentinelMetadataJSON() string {
	return MetadataJSON(SentinelLabels(), SentinelAnnotations())
}

// MetadataJSON encodes a metadata member value. A nil map omits that member.
func MetadataJSON(labels, annotations map[string]string) string {
	value := map[string]map[string]string{}
	if labels != nil {
		value["labels"] = labels
	}
	if annotations != nil {
		value["annotations"] = annotations
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// LabelsOfCount returns count distinct valid labels.
func LabelsOfCount(count int) map[string]string {
	labels := make(map[string]string, count)
	for i := range count {
		labels[fmt.Sprintf("example.com/label-%03d", i)] = fmt.Sprintf("value-%03d", i)
	}
	return labels
}

// AnnotationsOfDecodedSize returns annotations whose decoded keys and values
// total exactly size UTF-8 bytes. Values mix multi-byte characters and
// characters that JSON escapes, so the wire size differs from the decoded size.
func AnnotationsOfDecodedSize(size int) map[string]string {
	return annotationsOfDecodedSize(size, "東京é\n\"\\a")
}

// EscapedAnnotationsOfDecodedSize returns annotations of exactly size decoded
// bytes whose values are all control characters, each sent as a six-byte
// \u00XX escape. It is the largest valid annotation body on the wire.
func EscapedAnnotationsOfDecodedSize(size int) map[string]string {
	return annotationsOfDecodedSize(size, "\x01\x02\x1f")
}

func annotationsOfDecodedSize(size int, unit string) map[string]string {
	const perEntry = 8192
	annotations := map[string]string{}
	remaining := size
	for i := 0; remaining > 0; i++ {
		key := fmt.Sprintf("example.com/note-%03d", i)
		remaining -= len(key)
		if remaining < 0 {
			panic("annotation size too small for its keys")
		}
		budget := min(remaining, perEntry)
		var value strings.Builder
		for value.Len()+len(unit) <= budget {
			value.WriteString(unit)
		}
		for value.Len() < budget {
			value.WriteByte(unit[len(unit)-1])
		}
		annotations[key] = value.String()
		remaining -= value.Len()
	}
	return annotations
}

// DecodedMetadataSize sums the UTF-8 byte lengths of the keys and values.
func DecodedMetadataSize(entries map[string]string) int {
	total := 0
	for key, value := range entries {
		total += len(key) + len(value)
	}
	return total
}
