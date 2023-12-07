package core

import (
	"github.com/stretchr/testify/require"
	"slices"
	"testing"
)

func TestASCONEmptyKey(t *testing.T) {
	t.Run("seal", func(t *testing.T) {
		// output must match input
		input := []byte{1, 2, 3, 4, 5}
		asc, err := NewASCON(nil)
		require.NoError(t, err)
		output, err := asc.Seal(input)
		require.NoError(t, err)
		require.Equal(t, input, output)
	})

	t.Run("open", func(t *testing.T) {
		// output must match input
		input := []byte{1, 2, 3, 4, 5}
		asc, err := NewASCON(nil)
		require.NoError(t, err)
		output := asc.Open(input)
		require.NotNil(t, output)
		require.Equal(t, input, output)
	})
}

func TestASCONWithKey(t *testing.T) {
	sharedKey := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}
	otherKey := append([]byte{}, sharedKey...)
	slices.Reverse(otherKey)

	data := []byte{1, 2, 3, 4, 5}

	t.Run("round-trip", func(t *testing.T) {
		asc, err := NewASCON(sharedKey)
		require.NoError(t, err)

		// Perform a round-trip sealing and opening data.
		sealed, err := asc.Seal(data)
		require.NoError(t, err)
		require.NotEqual(t, sealed, data)

		open := asc.Open(sealed)
		require.NotNil(t, open)
		require.Equal(t, data, open)
	})

	t.Run("bad data", func(t *testing.T) {
		asc1, err := NewASCON(otherKey)
		require.NoError(t, err)
		asc2, err := NewASCON(sharedKey)
		require.NoError(t, err)

		sealed, err := asc1.Seal(data)
		require.NoError(t, err)

		open := asc2.Open(sealed)
		require.Nil(t, open)
	})
}
