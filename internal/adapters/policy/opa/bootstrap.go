package opa

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/fscontain"
)

const (
	maxBootstrapBytes = 1 << 20
	maxTokenBytes     = 64 << 10
	tokenPluginName   = "aib_bearer_token_file"
)

// Diagnostic reason classes. They name what is wrong, never a configured value.
const (
	reasonUnreadable              = "unreadable"
	reasonInvalidDocument         = "invalid_document"
	reasonUnsupportedKey          = "unsupported_key"
	reasonNoBundles               = "no_bundles"
	reasonPersistUnsupported      = "persist_unsupported"
	reasonInvalidOPAConfiguration = "invalid_opa_configuration"
	reasonPathNotAbsolute         = "path_not_absolute"
	reasonRootDirectory           = "root_directory"
	reasonEscapingLink            = "escaping_link"
	reasonFileTooLarge            = "file_too_large"
	reasonLocalFileUnsupported    = "local_file_unsupported"
	reasonTooManyFiles            = "too_many_files"
	reasonUnsupportedFile         = "unsupported_file"
	reasonTokenInvalid            = "token_invalid"
)

// configError is a credential-free engine configuration diagnostic: the configuration field, a
// reason class, and the offending key or relative file name.
type configError struct {
	field  string
	reason string
	key    string
	file   string
}

func (e *configError) Error() string {
	msg := e.field + ": " + e.reason
	if e.key != "" {
		msg += " key " + e.key
	}
	if e.file != "" {
		msg += " file " + e.file
	}
	return msg
}

// bootstrapConfig is a validated bootstrap: the original bytes for the OPA SDK, the configured
// bundle names, and the token-file plugin settings when configured.
type bootstrapConfig struct {
	raw       []byte
	bundles   []string
	tokenFile *tokenFileConfig
}

type tokenFileConfig struct {
	path   string
	scheme string
}

// readBootstrap reads the bootstrap through an os.Root over its own directory, bounded on the
// same descriptor, and validates it against the closed allowlist before any OPA code sees it.
func readBootstrap(path string) (*bootstrapConfig, error) {
	const field = "policy.config_file"
	raw, err := readContained(path, maxBootstrapBytes)
	if err != nil {
		return nil, &configError{field: field, reason: containmentReason(err)}
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, &configError{field: field, reason: reasonInvalidDocument}
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, &configError{field: field, reason: reasonInvalidDocument}
	}
	v := &bootstrapValidator{}
	v.document(root)
	if v.err != nil {
		v.err.field = field
		return nil, v.err
	}
	boot := &bootstrapConfig{raw: raw, bundles: v.bundles, tokenFile: v.tokenFile}
	if v.pluginReferenced && boot.tokenFile == nil {
		return nil, &configError{field: "plugins." + tokenPluginName + ".path", reason: reasonTokenInvalid}
	}
	if boot.tokenFile != nil {
		if _, err := readToken(boot.tokenFile.path); err != nil {
			return nil, err
		}
	}
	return boot, nil
}

// readContained reads one absolute file through an os.Root over the file's own directory.
func readContained(path string, max int64) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, &fscontain.Error{Reason: fscontain.ReasonNotAbsolute}
	}
	dir, err := fscontain.OpenDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	return dir.ReadFile(filepath.Base(path), max)
}

// readToken reads and trims the bundle token. Its errors never contain the token.
func readToken(path string) (string, error) {
	const field = "plugins." + tokenPluginName + ".path"
	raw, err := readContained(path, maxTokenBytes)
	if err != nil {
		return "", &configError{field: field, reason: containmentReason(err)}
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", &configError{field: field, reason: reasonTokenInvalid}
	}
	return token, nil
}

func containmentReason(err error) string {
	switch fscontain.ReasonOf(err) {
	case fscontain.ReasonNotAbsolute:
		return reasonPathNotAbsolute
	case fscontain.ReasonRootDirectory:
		return reasonRootDirectory
	case fscontain.ReasonEscapingLink:
		return reasonEscapingLink
	case fscontain.ReasonFileTooLarge:
		return reasonFileTooLarge
	case fscontain.ReasonTooManyFiles:
		return reasonTooManyFiles
	}
	return reasonUnreadable
}

// bootstrapValidator walks the bootstrap document and records the first violation.
type bootstrapValidator struct {
	err              *configError
	bundles          []string
	tokenFile        *tokenFileConfig
	pluginReferenced bool
}

func (v *bootstrapValidator) fail(reason, key string) {
	if v.err == nil {
		v.err = &configError{reason: reason, key: key}
	}
}

// members checks that obj is an object whose keys are all accepted.
func (v *bootstrapValidator) members(path string, value any, accepted ...string) map[string]any {
	obj, ok := value.(map[string]any)
	if !ok {
		v.fail(reasonInvalidDocument, path)
		return nil
	}
	allowed := make(map[string]bool, len(accepted))
	for _, key := range accepted {
		allowed[key] = true
	}
	for _, key := range sortedKeys(obj) {
		if !allowed[key] {
			v.fail(reasonUnsupportedKey, join(path, key))
		}
	}
	return obj
}

func (v *bootstrapValidator) document(root map[string]any) {
	v.members("", root, "services", "bundles", "keys", "labels", "plugins")
	if v.err != nil {
		return
	}
	if plugins, ok := root["plugins"]; ok {
		v.plugins(plugins)
	}
	if services, ok := root["services"]; ok {
		for name, service := range v.namedObjects("services", services) {
			v.service("services."+name, service)
		}
	}
	bundles := v.namedObjects("bundles", root["bundles"])
	if v.err == nil && len(bundles) == 0 {
		v.fail(reasonNoBundles, "")
	}
	for _, name := range sortedKeys(bundles) {
		v.bundle("bundles."+name, bundles[name])
		v.bundles = append(v.bundles, name)
	}
	if keys, ok := root["keys"]; ok {
		for name, key := range v.namedObjects("keys", keys) {
			v.members("keys."+name, key, "algorithm", "key", "scope")
		}
	}
	if labels, ok := root["labels"]; ok {
		v.stringMap("labels", labels)
	}
}

func (v *bootstrapValidator) namedObjects(path string, value any) map[string]any {
	if value == nil {
		return nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		v.fail(reasonInvalidDocument, path)
		return nil
	}
	return obj
}

func (v *bootstrapValidator) stringMap(path string, value any) {
	obj, ok := value.(map[string]any)
	if !ok {
		v.fail(reasonInvalidDocument, path)
		return
	}
	for _, key := range sortedKeys(obj) {
		if _, ok := obj[key].(string); !ok {
			v.fail(reasonInvalidDocument, join(path, key))
		}
	}
}

func (v *bootstrapValidator) plugins(value any) {
	plugins := v.members("plugins", value, tokenPluginName)
	settings, ok := plugins[tokenPluginName]
	if !ok {
		return
	}
	path := "plugins." + tokenPluginName
	obj := v.members(path, settings, "path", "scheme")
	if obj == nil {
		return
	}
	file, _ := obj["path"].(string)
	scheme, _ := obj["scheme"].(string)
	if scheme == "" {
		scheme = "Bearer"
	}
	if file == "" {
		v.fail(reasonTokenInvalid, path+".path")
		return
	}
	v.tokenFile = &tokenFileConfig{path: file, scheme: scheme}
}

func (v *bootstrapValidator) service(path string, value any) {
	service := v.members(path, value, "url", "headers", "credentials", "response_header_timeout_seconds",
		"tls", "client_tls")
	if service == nil {
		return
	}
	if _, ok := service["tls"]; ok {
		v.fail(reasonLocalFileUnsupported, path+".tls")
	}
	if _, ok := service["client_tls"]; ok {
		v.fail(reasonLocalFileUnsupported, path+".client_tls")
	}
	if url, _ := service["url"].(string); isFileURL(url) {
		v.fail(reasonLocalFileUnsupported, path+".url")
	}
	if headers, ok := service["headers"]; ok {
		v.stringMap(path+".headers", headers)
	}
	if credentials, ok := service["credentials"]; ok {
		v.credentials(path+".credentials", credentials)
	}
}

func (v *bootstrapValidator) credentials(path string, value any) {
	creds := v.members(path, value, "bearer", "oauth2", "s3_signing", "gcp_metadata", "azure_managed_identity", "plugin", "client_tls")
	if creds == nil {
		return
	}
	if _, ok := creds["client_tls"]; ok {
		v.fail(reasonLocalFileUnsupported, path+".client_tls")
		return
	}
	if len(creds) != 1 {
		v.fail(reasonInvalidDocument, path)
		return
	}
	for kind, settings := range creds {
		member := path + "." + kind
		switch kind {
		case "bearer":
			if obj, ok := settings.(map[string]any); ok {
				if _, ok := obj["token_path"]; ok {
					v.fail(reasonLocalFileUnsupported, member+".token_path")
					return
				}
			}
			v.members(member, settings, "token", "scheme")
		case "oauth2":
			if obj, ok := settings.(map[string]any); ok {
				if _, ok := obj["client_assertion_path"]; ok {
					v.fail(reasonLocalFileUnsupported, member+".client_assertion_path")
					return
				}
				if grant, ok := obj["grant_type"]; ok && grant != "client_credentials" {
					v.fail(reasonUnsupportedKey, member+".grant_type")
					return
				}
			}
			v.members(member, settings, "grant_type", "token_url", "client_id", "client_secret", "scopes",
				"additional_headers", "additional_parameters")
		case "s3_signing":
			v.s3Signing(member, settings)
		case "gcp_metadata":
			v.members(member, settings, "access_token_path", "audience", "endpoint", "identity_token_path", "scopes")
		case "azure_managed_identity":
			v.members(member, settings, "endpoint", "api_version", "resource", "object_id", "client_id", "mi_res_id", "use_app_service_msi")
		case "plugin":
			if settings != tokenPluginName {
				v.fail(reasonUnsupportedKey, member)
				return
			}
			v.pluginReferenced = true
		}
	}
}

func (v *bootstrapValidator) s3Signing(path string, value any) {
	if obj, ok := value.(map[string]any); ok {
		for _, fileReading := range []string{"web_identity_credentials", "profile_credentials", "sso_credentials"} {
			if _, ok := obj[fileReading]; ok {
				v.fail(reasonLocalFileUnsupported, path+"."+fileReading)
				return
			}
		}
	}
	signing := v.members(path, value, "environment_credentials", "metadata_credentials", "assume_role_credentials", "service", "signature_version")
	if signing == nil {
		return
	}
	if env, ok := signing["environment_credentials"]; ok && env != nil {
		v.members(path+".environment_credentials", env)
	}
	if metadata, ok := signing["metadata_credentials"]; ok {
		v.members(path+".metadata_credentials", metadata, "iam_role", "aws_region")
	}
	if assume, ok := signing["assume_role_credentials"]; ok {
		member := path + ".assume_role_credentials"
		obj := v.members(member, assume, "aws_region", "iam_role_arn", "session_name", "aws_domain", "aws_signing")
		if nested, ok := obj["aws_signing"]; ok {
			v.s3Signing(member+".aws_signing", nested)
		}
	}
}

func (v *bootstrapValidator) bundle(path string, value any) {
	bundle := v.members(path, value, "service", "resource", "signing", "polling", "size_limit_bytes", "trigger", "persist")
	if bundle == nil {
		return
	}
	if persist, ok := bundle["persist"]; ok && persist != false {
		v.fail(reasonPersistUnsupported, path+".persist")
	}
	if resource, _ := bundle["resource"].(string); isFileURL(resource) {
		v.fail(reasonLocalFileUnsupported, path+".resource")
	}
	if signing, ok := bundle["signing"]; ok {
		v.members(path+".signing", signing, "keyid", "scope", "exclude_files")
	}
	if polling, ok := bundle["polling"]; ok {
		v.members(path+".polling", polling, "min_delay_seconds", "max_delay_seconds", "long_polling_timeout_seconds")
	}
}

func isFileURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "file:")
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func sortedKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// reader returns the validated bootstrap bytes for the OPA SDK.
func (b *bootstrapConfig) reader() *bytes.Reader {
	return bytes.NewReader(b.raw)
}
