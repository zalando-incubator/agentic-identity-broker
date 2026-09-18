package cimd

import "testing"

func FuzzParseDocument(f *testing.F) {
	f.Add(validDoc(nil))
	f.Add([]byte(`{"client_id":"https://agent.example.com/client","redirect_uris":["https://agent.example.com/callback"]}`))
	f.Add([]byte(`{"client_id":"https://agent.example.com/client","redirect_uris":["https://agent.example.com/callback"],"client_secret":"secret"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		document, err := ParseDocument(data, fetchURL, []string{"blocked"})
		if err != nil {
			return
		}

		if document.ClientID != fetchURL {
			t.Fatalf("accepted document client_id = %q, want %q", document.ClientID, fetchURL)
		}
		if document.AuthMethod != "none" {
			t.Fatalf("accepted document auth method = %q, want none", document.AuthMethod)
		}
		if len(document.RedirectURIs) == 0 {
			t.Fatal("accepted document without redirect URIs")
		}
	})
}
