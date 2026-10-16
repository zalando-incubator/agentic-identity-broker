package cimd

import "testing"

func FuzzParseDocument(f *testing.F) {
	f.Add(validDoc(nil))
	f.Add([]byte(`{"client_id":"https://agent.example.com/client","redirect_uris":["https://agent.example.com/callback"]}`))
	f.Add([]byte(`{"client_id":"https://agent.example.com/client","redirect_uris":["https://agent.example.com/callback"],"client_secret":"secret"}`))
	f.Add(validDoc(map[string]any{
		"token_endpoint_auth_method": "private_key_jwt",
		"jwks_uri":                   "https://agent.example.com/keys/jwks.json",
	}))

	f.Fuzz(func(t *testing.T, data []byte) {
		document, err := ParseDocument(data, fetchURL, []string{"blocked"})
		if err != nil {
			return
		}

		if document.ClientID != fetchURL {
			t.Fatalf("accepted document client_id = %q, want %q", document.ClientID, fetchURL)
		}
		if document.AuthMethod != "none" && document.AuthMethod != "private_key_jwt" {
			t.Fatalf("accepted document auth method = %q, want none or private_key_jwt", document.AuthMethod)
		}
		if len(document.RedirectURIs) == 0 {
			t.Fatal("accepted document without redirect URIs")
		}
		if document.AuthMethod == "private_key_jwt" {
			if document.SigningAlg != "RS256" && document.SigningAlg != "ES256" {
				t.Fatalf("accepted private_key_jwt algorithm = %q", document.SigningAlg)
			}
			if (document.JWKSURI == "") == (len(document.JWKS) == 0) {
				t.Fatal("accepted private_key_jwt document without exactly one key source")
			}
		}
	})
}
