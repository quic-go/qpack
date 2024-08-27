package qpack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncoderMapHasValueForEveryStaticTableEntry(t *testing.T) {
	for idx, hf := range staticTableEntries {
		if len(hf.Value) == 0 {
			require.Equal(t, uint8(idx), encoderMap[hf.Name].idx)
		} else {
			require.Equal(t, uint8(idx), encoderMap[hf.Name].values[hf.Value])
		}
	}
}

func TestStaticTableasValueForEveryEncoderMapEntry(t *testing.T) {
	for name, indexAndVal := range encoderMap {
		if len(indexAndVal.values) == 0 {
			id := indexAndVal.idx
			require.Equal(t, name, staticTableEntries[id].Name)
			require.Empty(t, staticTableEntries[id].Value)
		} else {
			for value, id := range indexAndVal.values {
				require.Equal(t, name, staticTableEntries[id].Name)
				require.Equal(t, value, staticTableEntries[id].Value)
			}
		}
	}
}
