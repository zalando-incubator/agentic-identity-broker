package integration_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

type delegationSchema struct {
	Ref        string                       `yaml:"$ref"`
	Type       string                       `yaml:"type"`
	Properties map[string]*delegationSchema `yaml:"properties"`
	Items      *delegationSchema            `yaml:"items"`
	Enum       []string                     `yaml:"enum"`
}

func TestDelegationContractConformance(t *testing.T) {
	encoded, err := os.ReadFile("../../api/enduser/openapi.yaml")
	require.NoError(t, err)
	var document struct {
		Components struct {
			Schemas map[string]*delegationSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(encoded, &document))
	schemas := document.Components.Schemas

	t.Run("denial codes", func(t *testing.T) {
		codes := schemas["AccessEvaluationDenial"].Properties["context"].Properties["reason_admin"].Properties["code"].Enum
		require.NotEmpty(t, codes)
		constants := delegationStringConstants(t, "../../internal/domain/tokenexchange/diagnostic.go")
		domainCodes := delegationMapKeys(t, "../../internal/domain/tokenexchange/evaluation.go", "consentDenialDetails", constants)
		extprocCodes := delegationMapKeys(t, "../../internal/extproc/delegation/client.go", "allowlistedDenialCodes", nil)
		assert.ElementsMatch(t, codes, domainCodes, "domain denial set must match OpenAPI exactly")
		assert.ElementsMatch(t, codes, extprocCodes, "ExtProc allowlist must match OpenAPI exactly")
		for _, code := range codes {
			assert.True(t, tokenexchange.IsConsentDenialDetail(tokenexchange.FailureDetail(code)), code)
		}
		assert.False(t, tokenexchange.IsConsentDenialDetail("grant_unknown"))
	})

	t.Run("context fields", func(t *testing.T) {
		expected := make(map[string]bool)
		delegationSchemaFields(t, schemas["DelegationContext"], schemas, "", expected)
		for _, model := range []struct{ path, root string }{
			{"../../internal/domain/tokenexchange/evaluation.go", "DelegationContext"},
			{"../../internal/extproc/delegation/context.go", "wireContext"},
		} {
			t.Run(model.root, func(t *testing.T) {
				file := delegationParseFile(t, model.path)
				types := make(map[string]ast.Expr)
				ast.Inspect(file, func(node ast.Node) bool {
					if spec, ok := node.(*ast.TypeSpec); ok {
						types[spec.Name.Name] = spec.Type
					}
					return true
				})
				actual := make(map[string]bool)
				delegationModelFields(t, types[model.root], types, "", actual)
				assert.Equal(t, expected, actual, "all nested JSON field sets must match OpenAPI")
			})
		}
	})
}

func delegationParseFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)
	return file
}

func delegationStringConstants(t *testing.T, path string) map[string]string {
	t.Helper()
	values := make(map[string]string)
	ast.Inspect(delegationParseFile(t, path), func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		if literal, ok := spec.Values[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
			value, err := strconv.Unquote(literal.Value)
			require.NoError(t, err)
			values[spec.Names[0].Name] = value
		}
		return true
	})
	return values
}

// The private wire types and allowlists stay private: this cross-process contract
// check inspects their declarations instead of adding test-only production APIs.
func delegationMapKeys(t *testing.T, path, name string, constants map[string]string) []string {
	t.Helper()
	var keys []string
	ast.Inspect(delegationParseFile(t, path), func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != name {
			return true
		}
		require.Len(t, spec.Values, 1)
		literal, ok := spec.Values[0].(*ast.CompositeLit)
		require.True(t, ok)
		for _, element := range literal.Elts {
			entry, ok := element.(*ast.KeyValueExpr)
			require.True(t, ok)
			enabled, ok := entry.Value.(*ast.Ident)
			require.True(t, ok)
			require.Equal(t, "true", enabled.Name, "allowlist entries must be enabled")
			switch key := entry.Key.(type) {
			case *ast.Ident:
				value, exists := constants[key.Name]
				require.True(t, exists, "unresolved denial constant %s", key.Name)
				keys = append(keys, value)
			case *ast.BasicLit:
				value, err := strconv.Unquote(key.Value)
				require.NoError(t, err)
				keys = append(keys, value)
			default:
				t.Fatalf("unsupported denial key %T", key)
			}
		}
		return false
	})
	require.NotEmpty(t, keys, "missing allowlist %s", name)
	return keys
}

func delegationSchemaFields(t *testing.T, schema *delegationSchema, schemas map[string]*delegationSchema, prefix string, fields map[string]bool) {
	t.Helper()
	require.NotNil(t, schema)
	if schema.Ref != "" {
		schema = schemas[strings.TrimPrefix(schema.Ref, "#/components/schemas/")]
		require.NotNil(t, schema)
	}
	if schema.Type == "array" {
		delegationSchemaFields(t, schema.Items, schemas, prefix+"[]", fields)
		return
	}
	for name, child := range schema.Properties {
		path := prefix + "." + name
		fields[path] = true
		delegationSchemaFields(t, child, schemas, path, fields)
	}
}

func delegationModelFields(t *testing.T, expr ast.Expr, types map[string]ast.Expr, prefix string, fields map[string]bool) {
	t.Helper()
	switch node := expr.(type) {
	case *ast.StarExpr:
		delegationModelFields(t, node.X, types, prefix, fields)
	case *ast.ArrayType:
		delegationModelFields(t, node.Elt, types, prefix+"[]", fields)
	case *ast.Ident:
		if nested, ok := types[node.Name]; ok {
			delegationModelFields(t, nested, types, prefix, fields)
		}
	case *ast.StructType:
		for _, field := range node.Fields.List {
			require.NotNil(t, field.Tag)
			tag, err := strconv.Unquote(field.Tag.Value)
			require.NoError(t, err)
			name := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]
			require.NotEmpty(t, name)
			path := prefix + "." + name
			fields[path] = true
			delegationModelFields(t, field.Type, types, path, fields)
		}
	}
}
