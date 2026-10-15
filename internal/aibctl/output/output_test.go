package output_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

type item struct {
	ID          string  `json:"id"`
	CanonicalID *string `json:"canonical_id"`
	Name        string  `json:"display_name"`
}

func TestDisplayID(t *testing.T) {
	canon := "research-agent"
	empty := ""
	tests := []struct {
		name      string
		canonical *string
		want      string
	}{
		{"canonical present", &canon, "research-agent"},
		{"nil canonical", nil, "uuid-1"},
		{"empty canonical", &empty, "uuid-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, output.DisplayID(tc.canonical, "uuid-1"))
		})
	}
}

func TestPrintListTable(t *testing.T) {
	canon := "research-agent"
	items := []item{{ID: "uuid-1", CanonicalID: &canon, Name: "Research"}, {ID: "uuid-2", Name: "Plain"}}
	build := func(p *output.Printer) output.Table {
		t := output.Table{Header: []string{"ID", "NAME"}}
		if p.IDColumns() {
			t.Header = append(t.Header, "UUID")
		}
		for _, it := range items {
			row := []string{output.DisplayID(it.CanonicalID, it.ID), it.Name}
			if p.IDColumns() {
				row = append(row, it.ID)
			}
			t.Rows = append(t.Rows, row)
		}
		return t
	}

	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		p := &output.Printer{Out: &buf, Format: output.FormatTable}
		require.NoError(t, p.PrintList(items, func() output.Table { return build(p) }))
		assert.Equal(t, "ID              NAME\nresearch-agent  Research\nuuid-2          Plain\n", buf.String())
	})
	t.Run("show uuid", func(t *testing.T) {
		var buf bytes.Buffer
		p := &output.Printer{Out: &buf, Format: output.FormatTable, ShowUUID: true}
		require.NoError(t, p.PrintList(items, func() output.Table { return build(p) }))
		assert.Equal(t, "ID              NAME      UUID\nresearch-agent  Research  uuid-1\nuuid-2          Plain     uuid-2\n", buf.String())
	})
	t.Run("no headers", func(t *testing.T) {
		var buf bytes.Buffer
		p := &output.Printer{Out: &buf, Format: output.FormatTable, NoHeaders: true}
		require.NoError(t, p.PrintList(items, func() output.Table { return build(p) }))
		assert.Equal(t, "research-agent  Research\nuuid-2          Plain\n", buf.String())
	})
	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		p := &output.Printer{Out: &buf, Format: output.FormatJSON}
		require.NoError(t, p.PrintList(items, func() output.Table { return build(p) }))
		assert.JSONEq(t, `[{"id":"uuid-1","canonical_id":"research-agent","display_name":"Research"},{"id":"uuid-2","canonical_id":null,"display_name":"Plain"}]`, buf.String())
	})
	t.Run("yaml uses json field names", func(t *testing.T) {
		var buf bytes.Buffer
		p := &output.Printer{Out: &buf, Format: output.FormatYAML}
		require.NoError(t, p.PrintObject(items[0], nil))
		assert.Equal(t, "canonical_id: research-agent\ndisplay_name: Research\nid: uuid-1\n", buf.String())
	})
}

func TestPrintObjectKeyValue(t *testing.T) {
	var buf bytes.Buffer
	p := &output.Printer{Out: &buf, Format: output.FormatTable}
	require.NoError(t, p.PrintObject(nil, func() [][2]string {
		return [][2]string{{"ID", "research-agent"}, {"Display Name", "Research"}}
	}))
	assert.Equal(t, "ID:            research-agent\nDisplay Name:  Research\n", buf.String())
}
