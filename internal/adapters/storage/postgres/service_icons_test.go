package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceIconProviderRecordExactSourceAndIsolation(t *testing.T) {
	const source = "<svg xmlns=\"http://www.w3.org/2000/svg\">\r\n<text>Ä &amp; 東京 🗺</text></svg>\n"
	icon := source
	for _, tc := range []struct {
		name string
		icon *string
	}{
		{name: "absent"},
		{name: "exact source", icon: &icon},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entity := newTestEntity()
			entity.IconSVG, entity.IconSVGProvided = tc.icon, true
			record, err := entityToRecord(entity)
			require.NoError(t, err)
			if tc.icon != nil {
				*tc.icon = "input mutation"
			}
			stored, err := recordToEntity(record)
			require.NoError(t, err)
			assert.False(t, stored.IconSVGProvided)
			if tc.icon == nil {
				assert.Nil(t, stored.IconSVG)
				return
			}
			require.NotNil(t, stored.IconSVG)
			assert.Equal(t, []byte(source), []byte(*stored.IconSVG))
			*stored.IconSVG = "output mutation"
			again, err := recordToEntity(record)
			require.NoError(t, err)
			require.NotNil(t, again.IconSVG)
			assert.Equal(t, source, *again.IconSVG)
		})
	}
}
