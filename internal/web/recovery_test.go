package web

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/service"
)

func TestRecoverNeedsTheCodeFromTheDataFolder(t *testing.T) {
	srv, _ := newBlankServer(t, "")
	r := call(t, srv, http.MethodPost, "/api/setup", "", false, firstRun(""))
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	h := srv.Config.Handler.(*Server)
	codeFile := filepath.Join(t.TempDir(), "recovery-code.txt")

	body := func(code string) map[string]any {
		return map[string]any{"password": "a-new-synthetic-password", "code": code}
	}
	r = call(t, srv, http.MethodPost, "/api/recover", "", false, body("ABCD-EFGH-JKLM"))
	assert.Equal(t, http.StatusForbidden, r.status, "the server takes no resets without a code file set")

	h.SetRecoveryCodeFile(codeFile)
	r = call(t, srv, http.MethodPost, "/api/recover", "", false, body("ABCD-EFGH-JKLM"))
	assert.Equal(t, http.StatusForbidden, r.status, "no reset is waiting")
	assert.Contains(t, string(r.body), "kishtind reset-password")

	write := func(age time.Duration) {
		require.NoError(t, os.WriteFile(codeFile, []byte("ABCD-EFGH-JKLM\n"), 0o600))
		then := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(codeFile, then, then))
	}
	write(0)
	r = call(t, srv, http.MethodPost, "/api/recover", "", true, body("ABCD-EFGH-JKLM"))
	assert.Equal(t, http.StatusForbidden, r.status, "never from another device")
	assert.FileExists(t, codeFile, "a refused remote request does not use up the code")

	r = call(t, srv, http.MethodPost, "/api/recover", "", false, body("ABCD-EFGH-JKLN"))
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.NoFileExists(t, codeFile, "a wrong code uses it up")

	write(RecoveryCodeTTL + time.Minute)
	r = call(t, srv, http.MethodPost, "/api/recover", "", false, body("ABCD-EFGH-JKLM"))
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Contains(t, string(r.body), "expired")

	write(0)
	r = call(t, srv, http.MethodPost, "/api/recover", "", false, body("abcd efgh jklm"))
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	assert.Equal(t, "Máire Manager", decode[service.RecoveryResult](t, r).User.Name)
	assert.NoFileExists(t, codeFile, "the code works once")
	r = call(t, srv, http.MethodPost, "/api/recover", "", false, body("ABCD-EFGH-JKLM"))
	assert.Equal(t, http.StatusForbidden, r.status)
}
