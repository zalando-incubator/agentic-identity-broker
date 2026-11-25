package model

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const iconXMLWhitespace = " \t\r\n"

var (
	errIconDocument = errors.New("icon_svg must be a well-formed SVG document")
	errIconContent  = errors.New("icon_svg contains prohibited content")
)

func ValidateServiceIcon(source *string) error {
	if source == nil {
		return nil
	}
	if len(*source) == 0 || len(*source) > 65536 || !utf8.ValidString(*source) {
		return errors.New("icon_svg must contain 1 to 65536 UTF-8 bytes")
	}
	// The XML decoder does not check the character range in every token (for example, comments).
	for _, r := range *source {
		if !validXMLCharacter(r) {
			return errIconDocument
		}
	}
	document := strings.TrimPrefix(*source, "\ufeff")
	decoder := xml.NewDecoder(strings.NewReader(document))
	type elementScope struct {
		name             xml.Name
		previousBindings map[string]string
	}
	var elements []elementScope
	namespaces := make(iconNamespaces)
	rootSeen, doctypeSeen := false, false
	for {
		offset := decoder.InputOffset()
		token, err := decoder.RawToken()
		if err == io.EOF {
			if !rootSeen || len(elements) != 0 {
				return errIconDocument
			}
			return nil
		}
		if err != nil {
			return errIconDocument
		}
		switch token := token.(type) {
		case xml.StartElement:
			previousBindings, ok := namespaces.enter(token.Attr)
			if !ok {
				return errIconDocument
			}
			name, ok := expandIconName(token.Name, namespaces, true)
			if !ok {
				return errIconDocument
			}
			if len(elements) == 0 {
				if rootSeen || name.Local != "svg" || name.Space != "http://www.w3.org/2000/svg" {
					return errIconDocument
				}
				rootSeen = true
			}
			if err := validateIconElement(token, namespaces); err != nil {
				return err
			}
			elements = append(elements, elementScope{token.Name, previousBindings})
		case xml.EndElement:
			last := len(elements) - 1
			if last < 0 || elements[last].name != token.Name {
				return errIconDocument
			}
			namespaces.leave(elements[last].previousBindings)
			elements[last] = elementScope{}
			elements = elements[:last]
		case xml.CharData:
			if len(elements) == 0 && strings.Trim(document[offset:decoder.InputOffset()], iconXMLWhitespace) != "" {
				return errIconDocument
			}
		case xml.Directive:
			if rootSeen || doctypeSeen || !validIconDoctype(string(token)) {
				return errIconContent
			}
			doctypeSeen = true
		case xml.ProcInst:
			if offset != 0 || token.Target != "xml" || !validIconDeclaration(token.Inst) {
				return errIconContent
			}
		}
	}
}

// validXMLCharacter implements the XML 1.0 Char production for decoded runes.
func validXMLCharacter(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' ||
		r >= 0x20 && r <= 0xd7ff ||
		r >= 0xe000 && r <= 0xfffd ||
		r >= 0x10000 && r <= 0x10ffff
}

const (
	iconXMLNamespace   = "http://www.w3.org/XML/1998/namespace"
	iconXMLNSNamespace = "http://www.w3.org/2000/xmlns/"
)

type iconNamespaces map[string]string

func (active iconNamespaces) enter(attrs []xml.Attr) (map[string]string, bool) {
	var previous map[string]string
	for _, attr := range attrs {
		prefix := attr.Name.Local
		switch {
		case attr.Name.Space == "xmlns":
			if prefix == "" || prefix == "xmlns" || strings.Contains(prefix, ":") || attr.Value == "" {
				return nil, false
			}
		case attr.Name == (xml.Name{Local: "xmlns"}):
			prefix = ""
		default:
			continue
		}
		if attr.Value == iconXMLNSNamespace || (prefix == "xml") != (attr.Value == iconXMLNamespace) {
			return nil, false
		}
		if _, duplicate := previous[prefix]; duplicate {
			return nil, false
		}
		if previous == nil {
			previous = make(map[string]string)
		}
		previous[prefix] = active[prefix]
		active[prefix] = attr.Value
	}
	return previous, true
}

func (active iconNamespaces) leave(previous map[string]string) {
	for prefix, uri := range previous {
		if uri == "" {
			// For the default namespace only, an empty binding and no binding are equivalent.
			delete(active, prefix)
		} else {
			active[prefix] = uri
		}
	}
}

func expandIconName(name xml.Name, namespaces iconNamespaces, element bool) (xml.Name, bool) {
	if name.Local == "" || strings.Contains(name.Local, ":") || strings.Contains(name.Space, ":") {
		return name, false
	}
	if name.Space == "xml" {
		name.Space = iconXMLNamespace
		return name, true
	}
	if name.Space == "xmlns" {
		name.Space = iconXMLNSNamespace
		return name, !element
	}
	if name.Space == "" && !element {
		return name, true
	}
	if uri, ok := namespaces[name.Space]; ok {
		name.Space = uri
		return name, true
	}
	return name, name.Space == ""
}

func validateIconElement(element xml.StartElement, namespaces iconNamespaces) error {
	for _, name := range []string{"script", "a", "foreignobject"} {
		if iconASCIIEqual(element.Name.Local, name) {
			return errIconContent
		}
	}
	seen := make(map[xml.Name]struct{}, len(element.Attr))
	for _, attr := range element.Attr {
		name, ok := expandIconName(attr.Name, namespaces, false)
		if !ok {
			return errIconDocument
		}
		if _, duplicate := seen[name]; duplicate {
			return errIconDocument
		}
		seen[name] = struct{}{}
		local := attr.Name.Local
		if len(local) >= 2 && local[0]|0x20 == 'o' && local[1]|0x20 == 'n' || iconScriptURL(attr.Value) {
			return errIconContent
		}
	}
	return nil
}

func iconASCIIEqual(value, ascii string) bool {
	return len(value) == len(ascii) && strings.EqualFold(value, ascii)
}

func iconScriptURL(value string) bool {
	scheme, matched := "", 0
	for i := range len(value) {
		c := value[i]
		if c <= ' ' || c == 0x7f {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if matched == 0 {
			switch c {
			case 'j':
				scheme = "javascript:"
			case 'v':
				scheme = "vbscript:"
			default:
				return false
			}
		}
		if c != scheme[matched] {
			return false
		}
		matched++
		if matched == len(scheme) {
			return true
		}
	}
	return false
}

func iconXMLWord(value string) (string, string) {
	value = strings.TrimLeft(value, iconXMLWhitespace)
	if end := strings.IndexAny(value, iconXMLWhitespace); end >= 0 {
		return value[:end], value[end:]
	}
	return value, ""
}

func iconQuotedLiteral(value string) (string, bool) {
	trimmed := strings.TrimLeft(value, iconXMLWhitespace)
	if len(trimmed) == len(value) || len(trimmed) < 2 || trimmed[0] != '\'' && trimmed[0] != '"' {
		return "", false
	}
	end := strings.IndexByte(trimmed[1:], trimmed[0])
	if end < 0 {
		return "", false
	}
	return trimmed[end+2:], true
}

func validIconDoctype(value string) bool {
	word, rest := iconXMLWord(value)
	if word != "DOCTYPE" {
		return false
	}
	name, rest := iconXMLWord(rest)
	if name != "svg" && !strings.HasSuffix(name, ":svg") {
		return false
	}
	if strings.Trim(rest, iconXMLWhitespace) == "" {
		return true
	}
	kind, rest := iconXMLWord(rest)
	if kind != "SYSTEM" && kind != "PUBLIC" {
		return false
	}
	rest, ok := iconQuotedLiteral(rest)
	if !ok {
		return false
	}
	if kind == "PUBLIC" {
		rest, ok = iconQuotedLiteral(rest)
	}
	return ok && strings.Trim(rest, iconXMLWhitespace) == ""
}

func validIconDeclaration(instruction []byte) bool {
	decoder := xml.NewDecoder(io.MultiReader(strings.NewReader("<declaration "), bytes.NewReader(instruction), strings.NewReader("/>")))
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	element, ok := token.(xml.StartElement)
	if !ok || len(element.Attr) == 0 || element.Attr[0].Name != (xml.Name{Local: "version"}) || element.Attr[0].Value != "1.0" {
		return false
	}
	last := 0
	for _, attr := range element.Attr[1:] {
		switch {
		case last == 0 && attr.Name == (xml.Name{Local: "encoding"}) && iconASCIIEqual(attr.Value, "utf-8"):
			last = 1
		case last < 2 && attr.Name == (xml.Name{Local: "standalone"}) && (attr.Value == "yes" || attr.Value == "no"):
			last = 2
		default:
			return false
		}
	}
	if _, err := decoder.Token(); err != nil {
		return false
	}
	_, err = decoder.Token()
	return err == io.EOF
}
