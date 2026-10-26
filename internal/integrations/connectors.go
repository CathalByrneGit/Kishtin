package integrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Connector describes something Kishtin can connect to.
type Connector struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Summary says in a line what it does for the kitchen.
	Summary string `json:"summary"`
	// Sends says what leaves the building when it is used.
	Sends string `json:"sends"`
	// Environments are the accounts it can connect to, such as
	// production and sandbox.
	Environments []string `json:"environments"`
}

// Connectors lists every connector Kishtin has, in the order shown.
var Connectors = []Connector{
	{
		ID:           "square",
		Name:         "Square",
		Summary:      "Orders taken on the Square till come into the kitchen on their own. Kishtin only reads from Square.",
		Sends:        "The access token, the location ID and the time of the last order change seen. Nothing about the kitchen, staff or HACCP records.",
		Environments: []string{"production", "sandbox"},
	},
	{
		ID:           "sumup",
		Name:         "SumUp",
		Summary:      "Sales paid on the SumUp till come into the kitchen on their own, with their items. Suits counter service and takeaways, where people pay first. Kishtin only reads from SumUp.",
		Sends:        "The API key, the merchant code, the time of the last change seen, and the IDs of changed sales. Nothing about the kitchen, staff or HACCP records.",
		Environments: []string{"production"},
	},
}

// ConnectorByID finds a connector.
func ConnectorByID(id string) (Connector, bool) {
	i := slices.IndexFunc(Connectors, func(c Connector) bool { return c.ID == id })
	if i < 0 {
		return Connector{}, false
	}
	return Connectors[i], true
}

// Credentials is how to reach a connector's account.
type Credentials struct {
	Environment string `json:"environment"`
	Token       string `json:"token"`
}

// CredentialStore keeps connector credentials in files in one folder,
// readable by the server's user only. They are kept out of the database,
// so they are never in a backup, export or database dump.
type CredentialStore struct {
	Dir string
}

func (c CredentialStore) path(id string) (string, error) {
	if _, ok := ConnectorByID(id); !ok {
		return "", fmt.Errorf("no connector %q", id)
	}
	return filepath.Join(c.Dir, id+".json"), nil
}

// Load returns a connector's saved credentials, or false if there are none.
func (c CredentialStore) Load(id string) (Credentials, bool, error) {
	p, err := c.path(id)
	if err != nil {
		return Credentials{}, false, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, false, nil
	}
	if err != nil {
		return Credentials{}, false, fmt.Errorf("read %s credentials: %w", id, err)
	}
	var cr Credentials
	if err := json.Unmarshal(b, &cr); err != nil {
		return Credentials{}, false, fmt.Errorf("read %s credentials: %w", id, err)
	}
	return cr, cr.Token != "", nil
}

// Save writes a connector's credentials, replacing any saved before.
func (c CredentialStore) Save(id string, cr Credentials) error {
	p, err := c.path(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cr.Token) == "" {
		return errors.New("the access token is empty")
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return fmt.Errorf("save %s credentials: %w", id, err)
	}
	b, err := json.Marshal(cr)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("save %s credentials: %w", id, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("save %s credentials: %w", id, err)
	}
	return nil
}

// Delete removes a connector's saved credentials.
func (c CredentialStore) Delete(id string) error {
	p, err := c.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s credentials: %w", id, err)
	}
	return nil
}
