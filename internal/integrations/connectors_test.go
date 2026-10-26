package integrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialStore(t *testing.T) {
	st := CredentialStore{Dir: filepath.Join(t.TempDir(), "connectors")}
	_, ok, err := st.Load("square")
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, st.Save("square", Credentials{Environment: "sandbox", Token: "synthetic-token"}))
	info, err := os.Stat(filepath.Join(st.Dir, "square.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "only the server can read it")
	cr, ok, err := st.Load("square")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, Credentials{Environment: "sandbox", Token: "synthetic-token"}, cr)

	assert.Error(t, st.Save("square", Credentials{Token: " "}))
	assert.Error(t, st.Save("../etc", Credentials{Token: "x"}), "only known connectors")
	require.NoError(t, st.Delete("square"))
	require.NoError(t, st.Delete("square"), "deleting twice is fine")
	_, ok, err = st.Load("square")
	require.NoError(t, err)
	assert.False(t, ok)
}
