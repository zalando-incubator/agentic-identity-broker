package helpers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

// MaxLocalKeySetBytes mirrors the broker's 1 MiB bound for key-set documents.
const MaxLocalKeySetBytes = 1 << 20

// LocalKey is an in-memory signing key whose public half is published only as a local key set.
type LocalKey struct {
	KID string
	Alg jwa.SignatureAlgorithm

	private jwk.Key
}

// NewLocalES256Key generates a P-256 key that signs ES256.
func NewLocalES256Key(kid string) *LocalKey {
	raw, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("generate P-256 key: %v", err))
	}
	return newLocalKey(kid, jwa.ES256(), raw)
}

// NewLocalRS256Key generates a 2048-bit RSA key that signs RS256.
func NewLocalRS256Key(kid string) *LocalKey {
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generate RSA key: %v", err))
	}
	return newLocalKey(kid, jwa.RS256(), raw)
}

func newLocalKey(kid string, alg jwa.SignatureAlgorithm, raw any) *LocalKey {
	private, err := jwk.Import[jwk.Key](raw)
	if err != nil {
		panic(fmt.Sprintf("import key: %v", err))
	}
	mustSet(private, jwk.KeyIDKey, kid)
	mustSet(private, jwk.AlgorithmKey, alg)
	mustSet(private, jwk.KeyUsageKey, "sig")
	return &LocalKey{KID: kid, Alg: alg, private: private}
}

func mustSet(key jwk.Key, name string, value any) {
	if err := key.Set(name, value); err != nil {
		panic(fmt.Sprintf("set %s: %v", name, err))
	}
}

// Sign signs claims with the key; the JWS header carries the key's alg and kid.
func (k *LocalKey) Sign(claims map[string]any) (string, error) {
	tok := jwt.New()
	for name, value := range claims {
		if err := tok.Set(name, value); err != nil {
			return "", fmt.Errorf("set claim %q: %w", name, err)
		}
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(k.Alg, k.private))
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return string(signed), nil
}

// PublicMap returns the public JWK members of the key.
func (k *LocalKey) PublicMap() map[string]any {
	public, err := jwk.PublicKeyOf(k.private)
	if err != nil {
		panic(fmt.Sprintf("public key: %v", err))
	}
	return keyMap(public)
}

// PrivateMap returns every JWK member of the key, including private members.
func (k *LocalKey) PrivateMap() map[string]any {
	return keyMap(k.private)
}

// MemberValues returns the base64url x, y, n, and d values that tests assert never appear in output.
func (k *LocalKey) MemberValues() []string {
	var values []string
	for _, member := range []string{"x", "y", "n", "d"} {
		if value, ok := k.PrivateMap()[member].(string); ok && value != "" {
			values = append(values, value)
		}
	}
	return values
}

func keyMap(key jwk.Key) map[string]any {
	data, err := json.Marshal(key)
	if err != nil {
		panic(fmt.Sprintf("marshal key: %v", err))
	}
	var members map[string]any
	if err := json.Unmarshal(data, &members); err != nil {
		panic(fmt.Sprintf("unmarshal key: %v", err))
	}
	return members
}

func keySetJSON(members ...map[string]any) string {
	data, err := json.Marshal(map[string]any{"keys": members})
	if err != nil {
		panic(fmt.Sprintf("marshal key set: %v", err))
	}
	return string(data)
}

// PublicKeySet marshals the public keys as a compact JWK Set document.
func PublicKeySet(keys ...*LocalKey) string {
	members := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		members = append(members, key.PublicMap())
	}
	return keySetJSON(members...)
}

// PrivateKeySet marshals the key's private members into a JWK Set document, which the broker must reject.
func PrivateKeySet(key *LocalKey) string {
	return keySetJSON(key.PrivateMap())
}

// SymmetricKeySet returns a JWK Set holding one kty=oct key.
func SymmetricKeySet(kid string) string {
	secret := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	return keySetJSON(map[string]any{"kty": "oct", "kid": kid, "alg": "HS256", "k": secret})
}

// UnapprovedAlgorithmKeySet returns the public key with an alg outside the approved asymmetric set.
func UnapprovedAlgorithmKeySet(key *LocalKey, alg string) string {
	member := key.PublicMap()
	member["alg"] = alg
	return keySetJSON(member)
}

// DuplicateKIDKeySet returns two distinct public keys that share one kid.
func DuplicateKIDKeySet(first, second *LocalKey, kid string) string {
	firstMember, secondMember := first.PublicMap(), second.PublicMap()
	firstMember["kid"], secondMember["kid"] = kid, kid
	return keySetJSON(firstMember, secondMember)
}

// EmptyKeySet returns a JWK Set with no keys.
func EmptyKeySet() string {
	return `{"keys":[]}`
}

// BareJWK returns a single public JWK without the enclosing keys array.
func BareJWK(key *LocalKey) string {
	data, err := json.Marshal(key.PublicMap())
	if err != nil {
		panic(fmt.Sprintf("marshal bare key: %v", err))
	}
	return string(data)
}

// OversizedKeySet returns a syntactically valid JWK Set document larger than MaxLocalKeySetBytes.
func OversizedKeySet(key *LocalKey) string {
	member := key.PublicMap()
	member["x5t"] = strings.Repeat("a", MaxLocalKeySetBytes)
	return keySetJSON(member)
}

// TrustDir manages a per-test trust directory and the key-set files inside it.
type TrustDir struct {
	Path    string
	Outside string
}

// NewTrustDir creates an empty trust directory and a sibling directory outside of it.
// root must be a fresh temporary directory, for example GinkgoT().TempDir().
func NewTrustDir(root string) *TrustDir {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		panic(fmt.Sprintf("resolve temp dir: %v", err))
	}
	dir := &TrustDir{Path: filepath.Join(resolved, "trust"), Outside: filepath.Join(resolved, "outside")}
	for _, path := range []string{dir.Path, dir.Outside} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			panic(fmt.Sprintf("create %s: %v", path, err))
		}
	}
	return dir
}

// File returns the absolute path of name inside the trust directory.
func (d *TrustDir) File(name string) string {
	return filepath.Join(d.Path, name)
}

// WriteFile writes content as a plain file inside the trust directory.
func (d *TrustDir) WriteFile(name, content string) string {
	path := d.File(name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(fmt.Sprintf("write %s: %v", path, err))
	}
	return path
}

// WriteCorruptFile replaces name in place with a truncated document.
func (d *TrustDir) WriteCorruptFile(name string) string {
	return d.WriteFile(name, `{"keys":[`)
}

// WriteKubernetesLayout writes content in the layout a kubelet projects for a Secret or
// ConfigMap key: <name> -> ..data/<name> -> ..<generation>/<name>, using relative links only.
func (d *TrustDir) WriteKubernetesLayout(name, content, generation string) string {
	generationDir := filepath.Join(d.Path, ".."+generation)
	if err := os.MkdirAll(generationDir, 0o755); err != nil {
		panic(fmt.Sprintf("create %s: %v", generationDir, err))
	}
	if err := os.WriteFile(filepath.Join(generationDir, name), []byte(content), 0o644); err != nil {
		panic(fmt.Sprintf("write generation file: %v", err))
	}
	d.pointDataAt(".." + generation)
	nameLink := d.File(name)
	if _, err := os.Lstat(nameLink); os.IsNotExist(err) {
		if err := os.Symlink(filepath.Join("..data", name), nameLink); err != nil {
			panic(fmt.Sprintf("link %s: %v", nameLink, err))
		}
	}
	return nameLink
}

// SwapKubernetesLayout atomically moves ..data to a new generation holding content and
// removes the previous generation, as the kubelet does on a Secret update.
func (d *TrustDir) SwapKubernetesLayout(name, content, generation string) {
	previous, err := os.Readlink(filepath.Join(d.Path, "..data"))
	if err != nil {
		panic(fmt.Sprintf("read ..data link: %v", err))
	}
	d.WriteKubernetesLayout(name, content, generation)
	if err := os.RemoveAll(filepath.Join(d.Path, previous)); err != nil {
		panic(fmt.Sprintf("remove previous generation: %v", err))
	}
}

func (d *TrustDir) pointDataAt(target string) {
	temp := filepath.Join(d.Path, "..data_tmp")
	_ = os.Remove(temp)
	if err := os.Symlink(target, temp); err != nil {
		panic(fmt.Sprintf("link %s: %v", temp, err))
	}
	if err := os.Rename(temp, filepath.Join(d.Path, "..data")); err != nil {
		panic(fmt.Sprintf("rename ..data link: %v", err))
	}
}

// WriteOutsideFile writes content to ../outside/<name> beside the trust directory.
func (d *TrustDir) WriteOutsideFile(name, content string) string {
	path := filepath.Join(d.Outside, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(fmt.Sprintf("write %s: %v", path, err))
	}
	return path
}

// LinkRelativeEscape creates name inside the trust directory as a relative link to a file outside it.
func (d *TrustDir) LinkRelativeEscape(name, outsideName string) string {
	path := d.File(name)
	if err := os.Symlink(filepath.Join("..", "outside", outsideName), path); err != nil {
		panic(fmt.Sprintf("link %s: %v", path, err))
	}
	return path
}

// LinkAbsolute creates name inside the trust directory as an absolute link to target.
func (d *TrustDir) LinkAbsolute(name, target string) string {
	path := d.File(name)
	if err := os.Symlink(target, path); err != nil {
		panic(fmt.Sprintf("link %s: %v", path, err))
	}
	return path
}

// DotDotPath returns a path that starts inside the trust directory and escapes it through "..".
func (d *TrustDir) DotDotPath(outsideName string) string {
	return d.Path + "/../outside/" + outsideName
}
