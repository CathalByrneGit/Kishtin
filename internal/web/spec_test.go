package web

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The committed spec drives the generated CLI client, so it must match the
// routes. Run `make generate` after changing a route.
func TestCommittedSpecIsCurrent(t *testing.T) {
	want, err := Spec()
	require.NoError(t, err)
	got, err := os.ReadFile("../../api/openapi.json")
	require.NoError(t, err)
	assert.Equal(t, string(want)+"\n", string(got), "api/openapi.json is out of date: run make generate")
}
