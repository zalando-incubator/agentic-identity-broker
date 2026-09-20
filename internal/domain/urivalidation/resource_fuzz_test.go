package urivalidation

import (
	"net/url"
	"testing"
)

func FuzzNormalizeResourceURI(f *testing.F) {
	f.Add("https://api.example.com/resource///")
	f.Add("https://api.example.com/%2F/?target=/#section/")
	f.Add("not a URI")

	f.Fuzz(func(t *testing.T, resourceURI string) {
		normalized := NormalizeResourceURI(resourceURI)
		if got := NormalizeResourceURI(normalized); got != normalized {
			t.Fatalf("normalization is not idempotent: first %q, second %q", normalized, got)
		}

		original, err := url.Parse(resourceURI)
		if err != nil {
			return
		}
		parsedNormalized, err := url.Parse(normalized)
		if err != nil {
			t.Fatalf("normalization produced invalid URI %q: %v", normalized, err)
		}
		if parsedNormalized.RawQuery != original.RawQuery {
			t.Fatalf("normalization changed query from %q to %q", original.RawQuery, parsedNormalized.RawQuery)
		}
		if parsedNormalized.Fragment != original.Fragment {
			t.Fatalf("normalization changed fragment from %q to %q", original.Fragment, parsedNormalized.Fragment)
		}
	})
}
