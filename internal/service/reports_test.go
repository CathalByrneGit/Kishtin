package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
)

func TestReports(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	// HACCP: one fridge check this morning.
	_, err := k.svc.LogRecord(ctx, k.cook, fridge(3))
	require.NoError(t, err)

	// Pass: four burgers on a three-slot flat-top. The fourth waits and
	// reaches the pass late.
	var ids []string
	for i := range 4 {
		o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: string(rune('1' + i)), Items: []NewItem{{DishID: k.burger.ID}}})
		require.NoError(t, err)
		o, err = k.svc.FireCourse(ctx, k.chef, o.ID, menu.CourseMain)
		require.NoError(t, err)
		ids = append(ids, o.ID)
		if i < 3 {
			_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: []string{o.Items[0].ID}, From: orders.StateFired})
			require.NoError(t, err)
		}
	}
	_, err = k.svc.RecordPlan(ctx, "order")
	require.NoError(t, err)
	k.clk.Advance(11 * time.Minute)
	for _, id := range ids[:3] {
		_, err = k.svc.ReadyCourse(ctx, k.chef, id, menu.CourseMain)
		require.NoError(t, err)
		_, err = k.svc.ServeCourse(ctx, k.chef, id, menu.CourseMain)
		require.NoError(t, err)
	}
	o4, err := k.svc.GetOrder(ctx, k.chef, ids[3])
	require.NoError(t, err)
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: []string{o4.Items[0].ID}, From: orders.StateFired})
	require.NoError(t, err)
	k.clk.Advance(11 * time.Minute)
	_, err = k.svc.ReadyCourse(ctx, k.chef, ids[3], menu.CourseMain)
	require.NoError(t, err)

	pass, err := k.svc.ReportPass(ctx, k.chef, "", "")
	require.NoError(t, err)
	assert.Equal(t, 4, pass.Courses)
	assert.Equal(t, 3, pass.OnTime)
	assert.Equal(t, 1, pass.Late)
	require.Len(t, pass.LateCourses, 1)
	assert.Equal(t, scheduler.CauseEquipmentFull, pass.LateCourses[0].Cause, "the cause comes from the kept plan")
	assert.Equal(t, 3, pass.PassWait.Count, "the fourth was not served yet")
	_, err = k.svc.ReportPass(ctx, k.foh, "", "")
	assert.Equal(t, KindForbidden, KindOf(err))

	hr, err := k.svc.ReportHACCP(ctx, k.chef, "", "")
	require.NoError(t, err)
	assert.Equal(t, 1, hr.ChecksDone)
	assert.Equal(t, 1, hr.Records)
	assert.Len(t, hr.Temperatures, 3)
	_, err = k.svc.ReportHACCP(ctx, k.chef, "2026-05-02", "2026-05-01")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = k.svc.ReportHACCP(ctx, k.chef, "2025-01-01", "2026-05-01")
	assert.ErrorContains(t, err, "366 days at most")
}
