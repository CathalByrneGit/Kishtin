package diskfree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOf(t *testing.T) {
	s, err := Of(t.TempDir())
	require.NoError(t, err)
	assert.Positive(t, s.Total)
	assert.LessOrEqual(t, s.Free, s.Total)
	_, err = Of("/no/such/folder/for/kishtin")
	assert.Error(t, err)
}
