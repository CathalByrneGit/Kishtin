package web

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

// RecoveryCodeTTL is how long a code left by kishtind reset-password works.
const RecoveryCodeTTL = 10 * time.Minute

// SetRecoveryCodeFile names the file kishtind reset-password leaves its
// one-time code in. Without it, passwords cannot be reset while the server
// runs.
func (s *Server) SetRecoveryCodeFile(path string) { s.recoveryCode = path }

type recoverIn struct {
	Body struct {
		service.Recovery
		// Code is the one-time code kishtind reset-password left in the
		// data folder.
		Code string `json:"code"`
	}
}

type recoverOut struct {
	Body service.RecoveryResult
}

func (s *Server) recoveryRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "recover-manager", Method: http.MethodPost, Path: "/recover", Tags: []string{"Staff"},
		Summary: "Reset a manager's password from the Kishtin computer",
		Description: "Used by kishtind reset-password while the server runs. Works without an API key, only from the Kishtin computer itself, " +
			"and only with the one-time code that command leaves in the data folder, which is removed on first use and expires after 10 minutes. " +
			"With no active manager left, the named staff member is made a manager, or a new manager is added.",
		Security: []map[string][]string{},
		Metadata: map[string]any{publicKey: true},
	}, wrap(s, func(ctx context.Context, in *recoverIn) (*recoverOut, error) {
		if remote(ctx) {
			return nil, huma.Error403Forbidden("reset a password on the Kishtin computer itself: run kishtind reset-password there")
		}
		if err := s.useRecoveryCode(in.Body.Code); err != nil {
			return nil, err
		}
		res, err := s.svc.RecoverManager(ctx, in.Body.Recovery)
		if err != nil {
			return nil, err
		}
		return &recoverOut{Body: res}, nil
	}))
}

// useRecoveryCode checks a code against the file, and removes the file so
// the code works once.
func (s *Server) useRecoveryCode(code string) error {
	const how = "run kishtind reset-password on the Kishtin computer"
	if s.recoveryCode == "" {
		return huma.Error403Forbidden("this server does not take password resets")
	}
	fi, err := os.Stat(s.recoveryCode)
	if errors.Is(err, os.ErrNotExist) {
		return huma.Error403Forbidden("no password reset is waiting: " + how)
	}
	if err != nil {
		return err
	}
	b, err := os.ReadFile(s.recoveryCode)
	if err != nil {
		return err
	}
	if err := os.Remove(s.recoveryCode); err != nil {
		return err
	}
	if time.Since(fi.ModTime()) > RecoveryCodeTTL {
		return huma.Error403Forbidden("that reset code has expired: " + how + " again")
	}
	want := auth.NormalCode(strings.TrimSpace(string(b)))
	if want == "" || subtle.ConstantTimeCompare([]byte(auth.NormalCode(code)), []byte(want)) != 1 {
		return huma.Error403Forbidden("that reset code is wrong: " + how + " again")
	}
	return nil
}
