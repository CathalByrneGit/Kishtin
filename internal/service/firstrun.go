package service

import (
	"context"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

// FirstRun is what the setup screen asks for on a new install.
type FirstRun struct {
	// Restaurant is the name shown on every screen and the inspection pack.
	Restaurant string `json:"restaurant"`
	// Name, Password and PIN are the first manager's.
	Name     string `json:"name"`
	Password string `json:"password"`
	PIN      string `json:"pin,omitempty"`
	// Device names the screen doing the setup, which is paired with a new
	// device key.
	Device string `json:"device"`
}

// FirstRunResult is the first manager, a device key for the screen that set
// Kishtin up, and an API key for the kishtin command line. Both secrets are
// shown once.
type FirstRunResult struct {
	User      auth.User   `json:"user"`
	DeviceKey string      `json:"device_key"`
	APIKey    string      `json:"api_key"`
	Key       auth.APIKey `json:"key"`
}

// SetupNeeded reports whether Kishtin has no staff yet, so the setup screen
// should show.
func (s *Service) SetupNeeded(ctx context.Context) (bool, error) {
	users, err := s.st.Users().ListUsers(ctx)
	if err != nil {
		return false, err
	}
	return len(users) == 0, nil
}

// SetupFromScreen sets up a new install from the screens: the first manager,
// a device key for this screen, and the restaurant's name. It only works
// while there are no staff. The caller decides who may reach it.
func (s *Service) SetupFromScreen(ctx context.Context, in FirstRun) (FirstRunResult, error) {
	in.Restaurant = strings.TrimSpace(in.Restaurant)
	if in.Restaurant == "" {
		return FirstRunResult{}, newErr(KindInvalid, "type the restaurant's name, as it should show on screens and inspection packs")
	}
	if in.Password == "" {
		return FirstRunResult{}, newErr(KindInvalid, "the manager needs a password: it protects setup, limits and exports")
	}
	device := strings.TrimSpace(in.Device)
	if device == "" {
		device = "Setup screen"
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return FirstRunResult{}, err
	}
	next := sys.Value
	next.RestaurantName = in.Restaurant
	if err := next.Validate(); err != nil {
		return FirstRunResult{}, newErr(KindInvalid, "%s", err)
	}

	res, dev, err := s.setup(ctx, NewUser{Name: in.Name, Password: in.Password, PIN: in.PIN}, device)
	if err != nil {
		return FirstRunResult{}, err
	}
	p := &auth.Principal{User: res.User, Method: auth.MethodPassword, Device: dev.Key.ID, DeviceName: dev.Key.Name}
	if _, err := s.System().Save(ctx, p, SettingsChange[settings.System]{Value: next, Reason: "Restaurant name set at first setup"}); err != nil {
		return FirstRunResult{}, err
	}
	return FirstRunResult{User: res.User, DeviceKey: dev.Secret, APIKey: res.Secret, Key: res.Key}, nil
}
