package admin

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"slices"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

const metadataMember = "metadata"

// Messages are fixed so that no error can carry submitted text or decoder detail.
var (
	errMetadataMember               = errors.New("request body must contain at most one metadata member spelled exactly metadata")
	errMetadataNull                 = errors.New("metadata must not be null")
	errMetadataNotObject            = errors.New("metadata must be an object")
	errMetadataUnknownMember        = errors.New("metadata contains an unknown or repeated member")
	errMetadataLabelsNotObject      = errors.New("metadata.labels must be an object")
	errMetadataLabelsDuplicate      = errors.New("metadata.labels contains a duplicate key")
	errMetadataAnnotationsNotObject = errors.New("metadata.annotations must be an object")
	errMetadataAnnotationsDuplicate = errors.New("metadata.annotations contains a duplicate key")
)

// ObjectMetadataResponse presents labels and annotations. Empty maps are
// omitted, and encoding/json writes map keys in ascending byte order.
type ObjectMetadataResponse struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func objectMetadataResponse(metadata model.ObjectMetadata) *ObjectMetadataResponse {
	if metadata.IsEmpty() {
		return nil
	}
	return &ObjectMetadataResponse{Labels: metadata.Labels(), Annotations: metadata.Annotations()}
}

// decodeRequestMetadata reads the top-level metadata member of the first JSON
// value in body. Other members keep their existing decoding, so the scan
// tolerates invalid UTF-8 and duplicate names outside metadata. The returned
// error is always one of the fixed metadata errors.
func decodeRequestMetadata(body []byte) (model.ObjectMetadata, bool, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(body), jsontext.AllowInvalidUTF8(true), jsontext.AllowDuplicateNames(true))
	token, err := decoder.ReadToken()
	if err != nil {
		return model.ObjectMetadata{}, false, errMetadataMember
	}
	if token.Kind() != '{' {
		return model.ObjectMetadata{}, false, nil
	}
	var raw jsontext.Value
	present := false
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return model.ObjectMetadata{}, false, errMetadataMember
		}
		switch {
		case name.String() == metadataMember:
			if present {
				return model.ObjectMetadata{}, false, errMetadataMember
			}
			value, err := decoder.ReadValue()
			if err != nil {
				return model.ObjectMetadata{}, false, errMetadataMember
			}
			raw, present = value.Clone(), true
		case strings.EqualFold(name.String(), metadataMember):
			return model.ObjectMetadata{}, false, errMetadataMember
		default:
			if err := decoder.SkipValue(); err != nil {
				return model.ObjectMetadata{}, false, errMetadataMember
			}
		}
	}
	if !present {
		return model.ObjectMetadata{}, false, nil
	}
	metadata, err := parseMetadataValue(raw)
	if err != nil {
		return model.ObjectMetadata{}, true, err
	}
	return metadata, true, nil
}

// metadataWire is the strict shape of the metadata member. A null map is
// empty; a null entry value stays distinguishable so that it can be rejected.
type metadataWire struct {
	Labels      map[string]*string `json:"labels"`
	Annotations map[string]*string `json:"annotations"`
}

type metadataMapErrors struct {
	notObject, duplicate, invalidKey, invalidValue error
}

var metadataMaps = map[string]metadataMapErrors{
	"labels": {
		notObject: errMetadataLabelsNotObject, duplicate: errMetadataLabelsDuplicate,
		invalidKey: model.ErrMetadataLabelsInvalidKey, invalidValue: model.ErrMetadataLabelsInvalidValue,
	},
	"annotations": {
		notObject: errMetadataAnnotationsNotObject, duplicate: errMetadataAnnotationsDuplicate,
		invalidKey: model.ErrMetadataAnnotationsInvalidKey, invalidValue: model.ErrMetadataAnnotationsInvalidValue,
	},
}

// parseMetadataValue decodes metadata with encoding/json/v2, which rejects
// duplicate names, invalid UTF-8, unpaired surrogates, unknown members, and
// non-string values in document order. The domain constructor then applies
// the label rules before the annotation rules, each in ascending key order.
func parseMetadataValue(raw jsontext.Value) (model.ObjectMetadata, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return model.ObjectMetadata{}, errMetadataNull
	}
	var wire metadataWire
	if err := json.Unmarshal(raw, &wire, json.RejectUnknownMembers(true)); err != nil {
		return model.ObjectMetadata{}, metadataDecodeError(err)
	}
	labels, err := metadataEntries(wire.Labels, metadataMaps["labels"])
	if err != nil {
		return model.ObjectMetadata{}, err
	}
	annotations, err := metadataEntries(wire.Annotations, metadataMaps["annotations"])
	if err != nil {
		return model.ObjectMetadata{}, err
	}
	return model.NewObjectMetadata(labels, annotations)
}

func metadataEntries(wire map[string]*string, errs metadataMapErrors) (map[string]string, error) {
	entries := make(map[string]string, len(wire))
	for key, value := range wire {
		if value == nil {
			return nil, errs.invalidValue
		}
		entries[key] = *value
	}
	return entries, nil
}

// metadataDecodeError maps a decoder error to its fixed message from where it
// points: the metadata value, one of its maps, or one map entry. The decoder's
// own text, which quotes submitted names, is discarded.
func metadataDecodeError(err error) error {
	var pointer jsontext.Pointer
	var syntactic *jsontext.SyntacticError
	var semantic *json.SemanticError
	switch {
	case errors.As(err, &syntactic):
		pointer = syntactic.JSONPointer
	case errors.As(err, &semantic):
		pointer = semantic.JSONPointer
	default:
		return errMetadataNotObject
	}
	tokens := slices.Collect(pointer.Tokens())
	if len(tokens) == 0 {
		// The scan already read this value as valid JSON, so a syntactic
		// error here can only come from an undecodable member name.
		if syntactic != nil {
			return errMetadataUnknownMember
		}
		return errMetadataNotObject
	}
	errs, known := metadataMaps[tokens[0]]
	if !known || errors.Is(err, json.ErrUnknownName) || (len(tokens) == 1 && errors.Is(err, jsontext.ErrDuplicateName)) {
		return errMetadataUnknownMember
	}
	switch {
	case len(tokens) > 1 && errors.Is(err, jsontext.ErrDuplicateName):
		return errs.duplicate
	case len(tokens) > 1:
		return errs.invalidValue
	case syntactic != nil:
		return errs.invalidKey
	default:
		return errs.notObject
	}
}
