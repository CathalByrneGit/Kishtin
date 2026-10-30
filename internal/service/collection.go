package service

import (
	"context"
	"sort"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// CollectionOrder is a takeaway on the collection screen.
type CollectionOrder struct {
	// Number is the ticket number customers are told.
	Number int `json:"number"`
	// Name is the order's name, such as a customer's first name or a till
	// code. Screens show it only when set to.
	Name string `json:"name,omitempty"`
	// ReadyAt is when its last item reached the pass, once all have.
	ReadyAt *time.Time `json:"ready_at,omitempty"`
}

// CollectionBoard is what a screen facing customers shows: takeaways being
// prepared, and those ready to collect. It holds no items, notes or staff.
type CollectionBoard struct {
	Restaurant string            `json:"restaurant"`
	Preparing  []CollectionOrder `json:"preparing"`
	Ready      []CollectionOrder `json:"ready"`
}

// Collection returns the collection screen's board. It is for a paired
// screen facing customers, so it needs a device key but no staff login,
// and says only order numbers and names. An order leaves the board when
// it is served (collected), closed or voided.
func (s *Service) Collection(ctx context.Context) (CollectionBoard, error) {
	open, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
	if err != nil {
		return CollectionBoard{}, err
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return CollectionBoard{}, err
	}
	out := CollectionBoard{Restaurant: sys.Value.RestaurantName, Preparing: []CollectionOrder{}, Ready: []CollectionOrder{}}
	for _, o := range open {
		if o.Kind != orders.KindTakeaway {
			continue
		}
		var live, waiting, ready int
		var readyAt time.Time
		for _, it := range o.Items {
			switch it.State {
			case orders.StateVoided:
				continue
			case orders.StateServed:
			case orders.StateReady:
				ready++
				if it.ReadyAt != nil && it.ReadyAt.After(readyAt) {
					readyAt = *it.ReadyAt
				}
			default:
				waiting++
			}
			live++
		}
		c := CollectionOrder{Number: o.Number, Name: o.Name}
		switch {
		case live == 0 || (waiting == 0 && ready == 0):
			// Nothing to cook, or all collected.
		case waiting > 0:
			out.Preparing = append(out.Preparing, c)
		default:
			c.ReadyAt = &readyAt
			out.Ready = append(out.Ready, c)
		}
	}
	sort.SliceStable(out.Preparing, func(i, j int) bool { return out.Preparing[i].Number < out.Preparing[j].Number })
	sort.SliceStable(out.Ready, func(i, j int) bool { return out.Ready[i].ReadyAt.Before(*out.Ready[j].ReadyAt) })
	return out, nil
}
