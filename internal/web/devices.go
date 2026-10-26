package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/service"
)

type joinCodeIn struct {
	Body struct {
		// Name is what the tablet or phone will be called, such as
		// "Pass tablet".
		Name string `json:"name"`
	}
}

type joinCodeOut struct {
	Body service.JoinCode
}

type pairIn struct {
	Body struct {
		Code string `json:"code" minLength:"1"`
	}
}

type pairOut struct {
	Body service.Paired
}

func (s *Server) deviceRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "create-join-code", Method: http.MethodPost, Path: "/devices/join-codes", Tags: []string{"Keys"},
		Summary: "Make a QR code that pairs a tablet or phone",
		Description: "Makes a one-time code, good for 15 minutes, with a link and QR code for each address this computer has on the restaurant network. " +
			"A tablet that opens the link installs Kishtin's certificate if it needs to, then pairs itself with a new device key. Manager with a password.",
	}, wrap(s, func(ctx context.Context, in *joinCodeIn) (*joinCodeOut, error) {
		jc, err := s.svc.CreateJoinCode(ctx, principal(ctx), in.Body.Name)
		if err != nil {
			return nil, err
		}
		return &joinCodeOut{Body: jc}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "pair-device", Method: http.MethodPost, Path: "/devices/pair", Tags: []string{"Keys"},
		Summary: "Pair this tablet or phone with a join code",
		Description: "Works without an API key: the one-time code from a manager's QR code is the proof. " +
			"Returns a device key for this device, shown once.",
		Security: []map[string][]string{},
		Metadata: map[string]any{publicKey: true},
	}, wrap(s, func(ctx context.Context, in *pairIn) (*pairOut, error) {
		p, err := s.svc.Pair(ctx, in.Body.Code)
		if err != nil {
			return nil, err
		}
		return &pairOut{Body: p}, nil
	}))
}
