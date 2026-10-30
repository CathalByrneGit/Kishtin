package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

func TestCollection(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	takeaway := func(name string, course menu.Course, items ...NewItem) orders.Order {
		o, err := k.svc.CreateOrder(ctx, k.chef, NewOrder{Kind: orders.KindTakeaway, Name: name, Items: items, Fire: []menu.Course{course}})
		require.NoError(t, err)
		return o
	}
	move := func(o orders.Order, to ...orders.State) {
		for _, it := range o.Items {
			for _, st := range to {
				_, err := k.svc.MoveItems(ctx, k.chef, ItemMove{ItemIDs: []string{it.ID}, To: st})
				require.NoError(t, err)
			}
		}
	}
	_, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{{DishID: k.salad.ID}}})
	require.NoError(t, err)
	a := takeaway("Aoife", menu.CourseMain, NewItem{DishID: k.burger.ID})
	b := takeaway("", menu.CourseStarter, NewItem{DishID: k.salad.ID})
	c := takeaway("Synthetic customer", menu.CourseStarter, NewItem{DishID: k.salad.ID})

	board, err := k.svc.Collection(ctx)
	require.NoError(t, err)
	assert.Equal(t, []CollectionOrder{{Number: a.Number, Name: "Aoife"}, {Number: b.Number}, {Number: c.Number, Name: "Synthetic customer"}}, board.Preparing, "takeaways only, by number")
	assert.Empty(t, board.Ready)

	move(b, orders.StateCooking, orders.StatePlating, orders.StateReady)
	k.clk.Advance(1)
	move(c, orders.StateCooking, orders.StatePlating, orders.StateReady)
	board, err = k.svc.Collection(ctx)
	require.NoError(t, err)
	require.Len(t, board.Ready, 2)
	assert.Equal(t, b.Number, board.Ready[0].Number, "ready longest first")
	assert.NotNil(t, board.Ready[0].ReadyAt)
	assert.Len(t, board.Preparing, 1)

	move(b, orders.StateServed)
	board, err = k.svc.Collection(ctx)
	require.NoError(t, err)
	assert.Len(t, board.Ready, 1, "collected orders leave the board")
	assert.Equal(t, c.Number, board.Ready[0].Number)
}
