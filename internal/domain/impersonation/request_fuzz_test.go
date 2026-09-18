package impersonation

import (
	"net/url"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

func FuzzParseRequest(f *testing.F) {
	f.Add(url.Values(baseImpersonationForm()).Encode())
	f.Add("resource=&client_assertion=ca.jwt")

	f.Fuzz(func(t *testing.T, encodedForm string) {
		form, err := url.ParseQuery(encodedForm)
		if err != nil {
			return
		}
		_, err = ParseRequest(form)
		if err != nil {
			return
		}

		for _, parameter := range []string{
			tokenexchange.ClientAssertionParam,
			tokenexchange.ClientAssertionTypeParam,
			tokenexchange.ActorTokenParam,
			tokenexchange.ActorTokenTypeParam,
			tokenexchange.SubjectTokenParam,
			tokenexchange.SubjectTokenTypeParam,
		} {
			values := form[parameter]
			if len(values) != 1 || values[0] == "" {
				t.Fatalf("accepted form has invalid %s", parameter)
			}
		}
		if _, present := form[tokenexchange.ResourceParam]; present {
			t.Fatal("accepted impersonation request includes resource")
		}
	})
}
