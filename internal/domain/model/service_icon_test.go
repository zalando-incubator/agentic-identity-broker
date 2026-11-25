package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateServiceIcon(t *testing.T) {
	const root = `<svg xmlns="http://www.w3.org/2000/svg"`
	cases := []struct {
		name   string
		source string
		valid  bool
	}{
		{"minimal", root + `/>`, true},
		{"unicode exact source", "\n<!-- original -->\n" + root + `><text>Atlas café 世界 &amp; 星</text></svg>` + "\n", true},
		{"initial declaration", `<?xml version="1.0" encoding="UTF-8"?>` + root + `/>`, true},
		{"declaration without encoding", `<?xml version='1.0'?>` + root + `/>`, true},
		{"UTF8 BOM declaration", "\ufeff" + `<?xml version="1.0" encoding="UTF-8"?>` + root + `/>`, true},
		{"bare doctype", `<!DOCTYPE svg>` + root + `/>`, true},
		{"system doctype", `<!DOCTYPE svg SYSTEM "https://dtd.invalid/svg.dtd">` + root + `/>`, true},
		{"quoted system brackets", `<!DOCTYPE svg SYSTEM "https://dtd.invalid/[svg].dtd">` + root + `/>`, true},
		{"public doctype", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">` + root + `/>`, true},
		{"aliases and metadata", `<s:svg xmlns:s="http://www.w3.org/2000/svg" xmlns:m="urn:metadata"><m:description note="harmless">export</m:description><s:rect width="8" height="8"/></s:svg>`, true},
		{"prefix shadowing restored", root + ` xmlns:p="urn:outer" xmlns:q="urn:inner"><g xmlns:p="urn:inner"/><g p:width="8" q:width="9"/></svg>`, true},
		{"child namespace does not leak", root + `><g xmlns:p="urn:child"/><p:g/></svg>`, false},
		{"restored prefixes detect duplicate attributes", root + ` xmlns:p="urn:outer" xmlns:q="urn:outer"><g xmlns:p="urn:inner"/><g p:width="8" q:width="9"/></svg>`, false},
		{"nested default namespace reset", root + `><g xmlns=""><g xmlns="urn:other"/><g/></g><g/></svg>`, true},
		{"broad visual vocabulary", root + ` viewBox="0 0 64 64"><use href="#tile"/><defs><linearGradient id="paint"><stop stop-color="red"/></linearGradient><mask id="cut"><rect width="64" height="64" fill="white"/></mask><filter id="blur"><feGaussianBlur stdDeviation="1"/></filter><pattern id="pattern" width="2" height="2" patternUnits="userSpaceOnUse"><circle r="1"/></pattern><marker id="arrow"><path d="M0 0L2 1L0 2"/></marker><clipPath id="clip"><circle r="12"/></clipPath></defs><style>.tile{fill:url(#paint)}</style><rect id="tile" class="tile" mask="url(#cut)" filter="url(#blur)"><animate attributeName="opacity" values="0.5;1" dur="2s"/></rect><text font-family="serif">Atlas</text><image href="data:image/png;base64,iVBORw0KGgo="/></svg>`, true},
		{"inert external references", root + `><style>@import url(https://external.invalid/style.css);rect{fill:url(https://external.invalid/paint.svg#p)}</style><image href="https://external.invalid/media.png"/><use href="https://external.invalid/reuse.svg#tile"/><animate attributeName="href" values="https://external.invalid/a;https://external.invalid/b"/></svg>`, true},
		{"rendering concerns", root + `><use href="#missing"/><path id="same" d="browser-ignored"/><g id="same"/></svg>`, true},
		{"harmless active words", root + `><!-- <script/> --><metadata>foreignObject javascript: onload</metadata><text description="not javascript: leading">script</text></svg>`, true},
		{"valid XML comment characters", "<!--\t\n\r\u0020\ud7ff\ue000\ufffd\U00010000\U0010ffff-->" + root + `/>`, true},
		{"empty", "", false},
		{"whitespace", " \n\t", false},
		{"remote URL", "https://example.com/icon.svg", false},
		{"local path", "/icons/atlas.svg", false},
		{"invalid UTF8", root + ">\xff</svg>", false},
		{"NUL in comment", root + "><!--\x00--></svg>", false},
		{"control in system identifier", "<!DOCTYPE svg SYSTEM \"urn:\x0b\">" + root + `/>`, false},
		{"forbidden BMP noncharacters in comment", root + `/><!--` + "\ufffe\uffff" + `-->`, false},
		{"wrong namespace", `<svg xmlns="urn:other"/>`, false},
		{"no namespace", `<svg/>`, false},
		{"wrong root", `<g xmlns="http://www.w3.org/2000/svg"/>`, false},
		{"root case", `<SVG xmlns="http://www.w3.org/2000/svg"/>`, false},
		{"unclosed root", root + `>`, false},
		{"mismatched nesting", root + `><g></svg>`, false},
		{"two roots", root + `/>` + root + `/>`, false},
		{"leading text", `text` + root + `/>`, false},
		{"trailing text", root + `/>text`, false},
		{"trailing whitespace CDATA", root + `/><![CDATA[ ]]>`, false},
		{"leading character reference", `&#32;` + root + `/>`, false},
		{"undeclared element prefix", root + `><missing:g/></svg>`, false},
		{"undeclared attribute prefix", root + ` missing:width="8"/>`, false},
		{"invalid reserved prefix binding", root + ` xmlns:xml="urn:other"/>`, false},
		{"empty prefixed namespace", root + ` xmlns:p=""/>`, false},
		{"duplicate attributes", root + ` width="8" width="9"/>`, false},
		{"duplicate expanded attributes", root + ` xmlns:a="urn:same" xmlns:b="urn:same" a:width="8" b:width="9"/>`, false},
		{"unknown entity", root + `><text>&unknown;</text></svg>`, false},
		{"internal subset", `<!DOCTYPE svg []>` + root + `/>`, false},
		{"entity declaration", `<!DOCTYPE svg [<!ENTITY payload "hello">]>` + root + `/>`, false},
		{"external entity", `<!DOCTYPE svg [<!ENTITY payload SYSTEM "file:///etc/passwd">]>` + root + `><text>&payload;</text></svg>`, false},
		{"standalone entity directive", `<!ENTITY payload "hello">` + root + `/>`, false},
		{"other directive", `<!BOGUS svg>` + root + `/>`, false},
		{"doctype after root", root + `/><!DOCTYPE svg>`, false},
		{"doctype inside root", root + `><!DOCTYPE svg></svg>`, false},
		{"duplicate doctype", `<!DOCTYPE svg><!DOCTYPE svg>` + root + `/>`, false},
		{"non-svg doctype", `<!DOCTYPE html>` + root + `/>`, false},
		{"invalid doctype tail", `<!DOCTYPE svg garbage>` + root + `/>`, false},
		{"invalid system doctype", `<!DOCTYPE svg SYSTEM>` + root + `/>`, false},
		{"invalid public doctype", `<!DOCTYPE svg PUBLIC "identifier">` + root + `/>`, false},
		{"stylesheet instruction", `<?xml-stylesheet href="https://external.invalid/x.css"?>` + root + `/>`, false},
		{"instruction inside root", root + `><?export ready?></svg>`, false},
		{"declaration after whitespace", "\n<?xml version=\"1.0\"?>" + root + `/>`, false},
		{"duplicate declaration", `<?xml version="1.0"?><?xml version="1.0"?>` + root + `/>`, false},
		{"declaration after root", root + `/><?xml version="1.0"?>`, false},
		{"nonUTF8 declaration", `<?xml version="1.0" encoding="ISO-8859-1"?>` + root + `/>`, false},
		{"malformed declaration", `<?xml garbage?>` + root + `/>`, false},
		{"script", root + `><script>alert(1)</script></svg>`, false},
		{"case script", root + `><ScRiPt/></svg>`, false},
		{"foreign script", root + ` xmlns:x="urn:foreign"><x:script/></svg>`, false},
		{"hyperlink", root + `><a href="#tile"/></svg>`, false},
		{"foreign hyperlink", root + ` xmlns:x="urn:foreign"><x:A/></svg>`, false},
		{"foreign object", root + `><FOREIGNobject/></svg>`, false},
		{"root event", root + ` onload="alert(1)"/>`, false},
		{"descendant event", root + `><rect oNcLiCk="alert(1)"/></svg>`, false},
		{"namespaced event", root + ` xmlns:x="urn:foreign"><g x:ONLOAD="alert(1)"/></svg>`, false},
		{"on prefix", root + ` onlyVisual="yes"/>`, false},
		{"javascript attribute", root + `><image href="javascript:alert(1)"/></svg>`, false},
		{"decoded scheme", root + `><image href="&#x6a;AvA&#x09;sCrIpT&#x3a;alert(1)"/></svg>`, false},
		{"vbscript whitespace", root + `><g value=" &#x0a;VbScRiPt :msgbox(1)"/></svg>`, false},
		{"namespaced URL", root + ` xmlns:x="urn:foreign"><g x:value="javascript:alert(1)"/></svg>`, false},
		{"unicode visual name", root + `><scrİpt/></svg>`, true},
		{"invalid XML name", root + `><ſcript/></svg>`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.source
			err := ValidateServiceIcon(&tc.source)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "alert(1)")
				assert.NotContains(t, err.Error(), "file:///etc/passwd")
			}
			assert.Equal(t, before, tc.source, "validation must not rewrite source")
		})
	}
	t.Run("absent", func(t *testing.T) { require.NoError(t, ValidateServiceIcon(nil)) })
}

func TestValidateServiceIconDecodedByteLimit(t *testing.T) {
	const prefix = `<svg xmlns="http://www.w3.org/2000/svg"><text>`
	const suffix = `</text></svg>`
	for _, size := range []int{65535, 65536, 65537} {
		source := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			err := ValidateServiceIcon(&source)
			if size <= 65536 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	unicode := prefix + strings.Repeat("星", 22000) + suffix
	require.Error(t, ValidateServiceIcon(&unicode), "measure UTF-8 bytes, not code points")
	source := prefix + strings.Repeat("星", 20000) + suffix
	wire, err := json.Marshal(source)
	require.NoError(t, err)
	escaped := strings.ReplaceAll(string(wire), "星", `\u661f`)
	require.Greater(t, len(escaped), 65536)
	var decoded string
	require.NoError(t, json.Unmarshal([]byte(escaped), &decoded))
	require.NoError(t, ValidateServiceIcon(&decoded))
	assert.Equal(t, source, decoded)
}

func BenchmarkValidateServiceIconNested(b *testing.B) {
	for _, depth := range []int{1024, 8192} {
		source := `<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat("<g>", depth) + strings.Repeat("</g>", depth) + `</svg>`
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := ValidateServiceIcon(&source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
