package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// ActivityCustomerDetailsCleared is logged each time customers' names and
// order notes are cleared. It counts orders; it names no one.
const ActivityCustomerDetailsCleared = "orders.customer_details_cleared"

// clearBatch is how many orders are cleared in one transaction.
const clearBatch = 200

// maxPreviewCount caps how many orders a preview counts.
const maxPreviewCount = 100000

// CustomerDetailsPreview says what clearing customer details would do now.
type CustomerDetailsPreview struct {
	// Days is how many days after an order closes its details are kept;
	// 0 keeps them.
	Days int `json:"days"`
	// Before is the time orders must have closed before to be cleared.
	Before *time.Time `json:"before,omitempty"`
	// Orders is how many orders would be cleared now.
	Orders int `json:"orders"`
	// Notes say what is cleared and what is not.
	Notes []string `json:"notes"`
}

// CustomerDetailsResult is what clearing changed.
type CustomerDetailsResult struct {
	Before *time.Time `json:"before,omitempty"`
	// Orders is how many orders were cleared.
	Orders int `json:"orders"`
}

// customerDetailsNotes says, in plain words, what clearing does.
func customerDetailsNotes(days int) []string {
	if days <= 0 {
		return []string{
			"Customer details are kept: customer_details_days is 0 in system settings.",
			"Set a number of days to clear customers' names and order notes after that long.",
		}
	}
	return []string{
		fmt.Sprintf("Orders closed more than %d days ago lose their customer's name and their notes, and their items lose their notes. An order left open that long is cleared too.", days),
		"The same words are cleared from each order's history, from the copies kept of till orders, and from plans and voice replies kept while it was open.",
		"Tables, ticket numbers, dishes, times, allergy marks and who did what stay, so cook times and reports still work.",
		"Voice command transcripts, and the customer named on a HACCP delivery to a customer, are not cleared: HACCP records cannot be changed.",
		"Kishtin clears them every hour. This cannot be undone.",
	}
}

func (s *Service) customerDetailsCutoff(ctx context.Context) (int, time.Time, error) {
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return 0, time.Time{}, err
	}
	days := sys.Value.CustomerDetailsDays
	return days, s.clk.Now().AddDate(0, 0, -days), nil
}

// PreviewClearCustomerDetails shows how many orders would lose their
// customer details if they were cleared now.
func (s *Service) PreviewClearCustomerDetails(ctx context.Context, p *auth.Principal) (CustomerDetailsPreview, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return CustomerDetailsPreview{}, err
	}
	days, before, err := s.customerDetailsCutoff(ctx)
	if err != nil {
		return CustomerDetailsPreview{}, err
	}
	out := CustomerDetailsPreview{Days: days, Notes: customerDetailsNotes(days)}
	if days <= 0 {
		return out, nil
	}
	out.Before = &before
	ids, err := s.st.Orders().OrdersWithCustomerDetails(ctx, before, maxPreviewCount)
	if err != nil {
		return CustomerDetailsPreview{}, fmt.Errorf("find orders to clear: %w", err)
	}
	out.Orders = len(ids)
	return out, nil
}

// ClearCustomerDetails clears customers' names and order notes now from
// orders closed longer ago than system settings say to keep them.
func (s *Service) ClearCustomerDetails(ctx context.Context, p *auth.Principal) (CustomerDetailsResult, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return CustomerDetailsResult{}, err
	}
	return s.clearCustomerDetails(ctx, p)
}

// PurgeCustomerDetails is ClearCustomerDetails for the server's hourly
// job.
func (s *Service) PurgeCustomerDetails(ctx context.Context) (CustomerDetailsResult, error) {
	return s.clearCustomerDetails(ctx, nil)
}

func (s *Service) clearCustomerDetails(ctx context.Context, p *auth.Principal) (CustomerDetailsResult, error) {
	days, before, err := s.customerDetailsCutoff(ctx)
	if err != nil || days <= 0 {
		return CustomerDetailsResult{}, err
	}
	out := CustomerDetailsResult{Before: &before}
	for {
		ids, err := s.st.Orders().OrdersWithCustomerDetails(ctx, before, clearBatch)
		if err != nil {
			return out, fmt.Errorf("find orders to clear: %w", err)
		}
		if len(ids) == 0 {
			break
		}
		err = s.st.WithTx(ctx, func(tx store.Store) error {
			for _, id := range ids {
				if err := s.clearOrder(ctx, tx, id); err != nil {
					return fmt.Errorf("clear order %s: %w", id, err)
				}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
		out.Orders += len(ids)
		if len(ids) < clearBatch {
			break
		}
	}
	if out.Orders == 0 {
		return out, nil
	}
	if err := s.note(ctx, s.st, p, ActivityCustomerDetailsCleared, "", map[string]any{
		"orders": out.Orders, "before": before, "days": days,
	}); err != nil {
		return out, err
	}
	s.publish(events.KindOrder, "")
	return out, nil
}

// clearOrder clears one order's customer details, with every name it has
// had and every note it and its items held.
func (s *Service) clearOrder(ctx context.Context, tx store.Store, id string) error {
	o, err := tx.Orders().GetOrder(ctx, id)
	if err != nil {
		return err
	}
	evs, err := tx.Orders().ListEvents(ctx, id)
	if err != nil {
		return err
	}
	c := store.CustomerClear{OrderID: id, From: o.CreatedAt, To: s.clk.Now()}
	if o.ClosedAt != nil {
		c.To = *o.ClosedAt
	}
	c.Names = appendSet(c.Names, o.Name)
	for _, e := range evs {
		if e.Kind == orders.EventRenamed {
			c.Names = appendSet(c.Names, e.From, e.To)
		}
	}
	c.Notes = appendSet(c.Notes, o.Notes)
	for _, it := range o.Items {
		c.Notes = appendSet(c.Notes, it.Notes)
	}
	return tx.Orders().ClearCustomerDetails(ctx, c)
}

// appendSet appends the non-empty values not already in list.
func appendSet(list []string, vs ...string) []string {
	for _, v := range vs {
		if v == "" {
			continue
		}
		seen := false
		for _, have := range list {
			if have == v {
				seen = true
				break
			}
		}
		if !seen {
			list = append(list, v)
		}
	}
	return list
}
