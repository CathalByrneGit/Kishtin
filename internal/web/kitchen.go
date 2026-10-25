package web

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

// out wraps a response body.
type out[T any] struct {
	Body T
}

func reply[T any](v T, err error) (*out[T], error) {
	if err != nil {
		return nil, err
	}
	return &out[T]{Body: v}, nil
}

type idIn[T any] struct {
	ID   string `path:"id"`
	Body T
}

type courseIn[T any] struct {
	ID     string      `path:"id"`
	Course menu.Course `path:"course" enum:"starter,main,dessert"`
	Body   T
}

type courseOnly struct {
	ID     string      `path:"id"`
	Course menu.Course `path:"course" enum:"starter,main,dessert"`
}

type availabilityBody struct {
	Available bool `json:"available"`
}

type addItemsBody struct {
	Items []service.NewItem `json:"items" minItems:"1"`
}

type targetBody struct {
	// TargetAt is the new time the course should reach the pass.
	TargetAt time.Time `json:"target_at"`
	Reason   string    `json:"reason,omitempty"`
}

type holdBody struct {
	Reason string `json:"reason,omitempty"`
}

type holdIn struct {
	ID     string      `path:"id"`
	Course menu.Course `path:"course" enum:"starter,main,dessert"`
	Body   holdBody    `required:"false"`
}

type urgentBody struct {
	Urgent bool `json:"urgent"`
}

type ordersBody struct {
	Orders []orders.Order `json:"orders"`
}

type eventsBody struct {
	Events []orders.Event `json:"events"`
}

func (s *Server) kitchenRoutes() {
	menuTags := []string{"Menu"}
	huma.Register(s.api, huma.Operation{
		OperationID: "get-menu", Method: http.MethodGet, Path: "/menu", Tags: menuTags,
		Summary: "Show stations, equipment and dishes",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[menu.Setup], error) {
		return reply(s.svc.Menu(ctx, principal(ctx)))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "create-station", Method: http.MethodPost, Path: "/menu/stations", Tags: menuTags,
		Summary: "Add a station (manager password)", DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.StationInput]) (*out[menu.Station], error) {
		return reply(s.svc.CreateStation(ctx, principal(ctx), in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "update-station", Method: http.MethodPut, Path: "/menu/stations/{id}", Tags: menuTags,
		Summary:     "Change a station (manager password)",
		Description: "Set active to false to take it out of use. Stations are never deleted, because past orders refer to them.",
	}, wrap(s, func(ctx context.Context, in *idIn[service.StationInput]) (*out[menu.Station], error) {
		return reply(s.svc.UpdateStation(ctx, principal(ctx), in.ID, in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "create-equipment", Method: http.MethodPost, Path: "/menu/equipment", Tags: menuTags,
		Summary: "Add equipment to a station (manager password)", DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.EquipmentInput]) (*out[menu.Equipment], error) {
		return reply(s.svc.CreateEquipment(ctx, principal(ctx), in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "update-equipment", Method: http.MethodPut, Path: "/menu/equipment/{id}", Tags: menuTags,
		Summary: "Change equipment (manager password)",
	}, wrap(s, func(ctx context.Context, in *idIn[service.EquipmentInput]) (*out[menu.Equipment], error) {
		return reply(s.svc.UpdateEquipment(ctx, principal(ctx), in.ID, in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "create-dish", Method: http.MethodPost, Path: "/menu/dishes", Tags: menuTags,
		Summary: "Add a dish (manager password)", DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.DishInput]) (*out[menu.Dish], error) {
		return reply(s.svc.CreateDish(ctx, principal(ctx), in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "update-dish", Method: http.MethodPut, Path: "/menu/dishes/{id}", Tags: menuTags,
		Summary:     "Change a dish (manager password)",
		Description: "Replaces every field. Orders already in the kitchen keep the times they were ordered with.",
	}, wrap(s, func(ctx context.Context, in *idIn[service.DishInput]) (*out[menu.Dish], error) {
		return reply(s.svc.UpdateDish(ctx, principal(ctx), in.ID, in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "set-dish-availability", Method: http.MethodPut, Path: "/menu/dishes/{id}/availability", Tags: menuTags,
		Summary: "Mark a dish sold out (86) or back on",
	}, wrap(s, func(ctx context.Context, in *idIn[availabilityBody]) (*out[menu.Dish], error) {
		return reply(s.svc.SetAvailability(ctx, principal(ctx), in.ID, in.Body.Available))
	}))

	orderTags := []string{"Orders"}
	huma.Register(s.api, huma.Operation{
		OperationID: "list-orders", Method: http.MethodGet, Path: "/orders", Tags: orderTags,
		Summary: "List open orders, or every order of a day",
	}, wrap(s, func(ctx context.Context, in *struct {
		Date string `query:"date" doc:"A day (YYYY-MM-DD, Dublin time). Leave out for open orders."`
	}) (*out[ordersBody], error) {
		list, err := s.svc.ListOrders(ctx, principal(ctx), service.OrderFilter{Date: in.Date})
		return reply(ordersBody{Orders: list}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "create-order", Method: http.MethodPost, Path: "/orders", Tags: orderTags,
		Summary:       "Enter an order",
		Description:   "Courses are held until the expo fires them. List courses in fire to fire them straight away (head chef or manager).",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *out[service.NewOrder]) (*out[orders.Order], error) {
		return reply(s.svc.CreateOrder(ctx, principal(ctx), in.Body))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "get-order", Method: http.MethodGet, Path: "/orders/{id}", Tags: orderTags,
		Summary: "Show an order",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*out[orders.Order], error) {
		return reply(s.svc.GetOrder(ctx, principal(ctx), in.ID))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "order-events", Method: http.MethodGet, Path: "/orders/{id}/events", Tags: orderTags,
		Summary: "Show an order's history: every change, who made it and when",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*out[eventsBody], error) {
		evs, err := s.svc.OrderEvents(ctx, principal(ctx), in.ID)
		return reply(eventsBody{Events: evs}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "add-items", Method: http.MethodPost, Path: "/orders/{id}/items", Tags: orderTags,
		Summary: "Add items to an open order",
	}, wrap(s, func(ctx context.Context, in *idIn[addItemsBody]) (*out[orders.Order], error) {
		return reply(s.svc.AddItems(ctx, principal(ctx), in.ID, in.Body.Items))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "fire-course", Method: http.MethodPost, Path: "/orders/{id}/courses/{course}/fire", Tags: orderTags,
		Summary: "Fire a course: its target time is now plus its longest item",
	}, wrap(s, func(ctx context.Context, in *courseOnly) (*out[orders.Order], error) {
		return reply(s.svc.FireCourse(ctx, principal(ctx), in.ID, in.Course))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "hold-course", Method: http.MethodPost, Path: "/orders/{id}/courses/{course}/hold", Tags: orderTags,
		Summary:     "Hold a fired course again (un-fire it)",
		Description: "Only while nothing in the course has started. Its items leave the station queues and its target time is cleared. Logged with the reason.",
	}, wrap(s, func(ctx context.Context, in *holdIn) (*out[orders.Order], error) {
		return reply(s.svc.HoldCourse(ctx, principal(ctx), in.ID, in.Course, in.Body.Reason))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "serve-course", Method: http.MethodPost, Path: "/orders/{id}/courses/{course}/serve", Tags: orderTags,
		Summary: "Serve a course once every item is ready",
	}, wrap(s, func(ctx context.Context, in *courseOnly) (*out[orders.Order], error) {
		return reply(s.svc.ServeCourse(ctx, principal(ctx), in.ID, in.Course))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "set-course-target", Method: http.MethodPut, Path: "/orders/{id}/courses/{course}/target", Tags: orderTags,
		Summary:     "Change when a fired course should reach the pass",
		Description: "An expo override. It is logged with its reason and the scheduler plans around it.",
	}, wrap(s, func(ctx context.Context, in *courseIn[targetBody]) (*out[orders.Order], error) {
		return reply(s.svc.SetTarget(ctx, principal(ctx), in.ID, in.Course, in.Body.TargetAt, in.Body.Reason))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "set-course-urgent", Method: http.MethodPut, Path: "/orders/{id}/courses/{course}/urgent", Tags: orderTags,
		Summary: "Flag a course urgent so the scheduler puts it first, or clear the flag",
	}, wrap(s, func(ctx context.Context, in *courseIn[urgentBody]) (*out[orders.Order], error) {
		return reply(s.svc.SetUrgent(ctx, principal(ctx), in.ID, in.Course, in.Body.Urgent))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "close-order", Method: http.MethodPost, Path: "/orders/{id}/close", Tags: orderTags,
		Summary: "Close an order once everything is served or voided",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*out[orders.Order], error) {
		return reply(s.svc.CloseOrder(ctx, principal(ctx), in.ID))
	}))

	kitchenTags := []string{"Kitchen"}
	huma.Register(s.api, huma.Operation{
		OperationID: "get-kitchen", Method: http.MethodGet, Path: "/kitchen", Tags: kitchenTags,
		Summary:     "Show open orders and the scheduler's plan",
		Description: "The plan is made fresh on every request: what each station should start and when, why, and whether each fired course is on track, at risk or late.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[service.KitchenView], error) {
		return reply(s.svc.Kitchen(ctx, principal(ctx)))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "move-items", Method: http.MethodPost, Path: "/kitchen/items", Tags: kitchenTags,
		Summary:     "Move items: started, rested, plated, ready, served or voided",
		Description: "Leave out to to move each item one step forward. Items can belong to different orders. Voiding needs a reason. Cooks move items through cooking; the head chef or a manager serves and voids.",
	}, wrap(s, func(ctx context.Context, in *out[service.ItemMove]) (*out[ordersBody], error) {
		list, err := s.svc.MoveItems(ctx, principal(ctx), in.Body)
		return reply(ordersBody{Orders: list}, err)
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "replace-item", Method: http.MethodPost, Path: "/kitchen/items/{id}/replace", Tags: kitchenTags,
		Summary:     "Replace an item with the right one: void it and add the new one in one step",
		Description: "The new item joins the same course and goes straight to its station if the course is fired. Front of house can replace items in a held course; once fired, the head chef or a manager.",
	}, wrap(s, func(ctx context.Context, in *idIn[service.ItemReplace]) (*out[orders.Order], error) {
		return reply(s.svc.ReplaceItem(ctx, principal(ctx), in.ID, in.Body))
	}))

	registerGroup(s, settings.KindScheduler, "scheduler settings",
		"The cook scheduler's strategy, batching window, priorities, station buffers and re-plan interval. Every plan says which version produced it.",
		func() *service.Group[scheduler.Settings] { return s.svc.Scheduler() })
}
