package model

import (
	"encoding/json"
	"strings"
	"testing"
)

const fuzzPrivateKeyMarker = "fuzz-private-key-material"

func FuzzParseGoogleServiceAccountKey(f *testing.F) {
	f.Add(validGoogleServiceAccountJSON)
	f.Add(`{"type":"service_account","private_key":"` + fuzzPrivateKeyMarker + `","client_email":"fuzz@example.com","token_uri":"https://oauth2.googleapis.com/token","client_id":"123"}`)
	f.Add(strings.Repeat("x", maxGoogleServiceAccountKeySize+1))

	f.Fuzz(func(t *testing.T, credential string) {
		key, err := ParseGoogleServiceAccountKey(credential)
		if len(credential) > maxGoogleServiceAccountKeySize {
			if err == nil {
				t.Fatal("oversized credential was accepted")
			}
			return
		}
		if err != nil {
			var fields struct {
				PrivateKey string `json:"private_key"`
			}
			if json.Unmarshal([]byte(credential), &fields) == nil && fields.PrivateKey == fuzzPrivateKeyMarker && strings.Contains(err.Error(), fields.PrivateKey) {
				t.Fatal("credential parser exposed private key material in an error")
			}
			return
		}

		if key.Type != "service_account" || key.ClientEmail == "" || len(key.PrivateKey) == 0 || key.TokenURI == "" || key.ClientID == "" {
			t.Fatal("accepted credential omitted a required service account field")
		}
	})
}
