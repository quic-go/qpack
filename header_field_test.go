package qpack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeaderFieldIsPseudo(t *testing.T) {
	t.Run("Pseudo headers", func(t *testing.T) {
		require.True(t, (HeaderField{Name: ":status"}).IsPseudo())
		require.True(t, (HeaderField{Name: ":authority"}).IsPseudo())
		require.True(t, (HeaderField{Name: ":foobar"}).IsPseudo())
	})

	t.Run("Non-pseudo headers", func(t *testing.T) {
		require.False(t, (HeaderField{Name: "status"}).IsPseudo())
		require.False(t, (HeaderField{Name: "foobar"}).IsPseudo())
	})
}
