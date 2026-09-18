package impersonation

import (
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwa"
)

func FuzzParseUnverifiedSubject(f *testing.F) {
	f.Add("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJjaGF0LXVzZXItMSJ9.")
	f.Add("eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJjaGF0LXVzZXItMSJ9.c2lnbmF0dXJl")
	f.Add("not-a-jwt")

	f.Fuzz(func(t *testing.T, token string) {
		_, err := parseUnverifiedSubject(token)
		if err != nil {
			return
		}

		signature, err := soleSignature(token)
		if err != nil {
			t.Fatalf("accepted unverified subject has no parseable signature: %v", err)
		}
		if len(signature.Signature()) != 0 {
			t.Fatal("accepted unverified subject has a signature")
		}
		if algorithm, ok := signature.ProtectedHeaders().Algorithm(); !ok || algorithm != jwa.NoSignature() {
			t.Fatalf("accepted unverified subject algorithm = %q, want none", algorithm)
		}
	})
}
