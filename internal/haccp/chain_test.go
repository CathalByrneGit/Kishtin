package haccp

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chain(t *testing.T, n int) []Record {
	t.Helper()
	t0 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	var out []Record
	var prev *Record
	for k := range n {
		r := Seal(Record{
			ID:            fmt.Sprintf("rec-%d", k),
			Type:          RecordFridge,
			LoggedBy:      "user-1",
			LoggedByName:  "Aoife (synthetic)",
			LoggedAt:      t0.Add(time.Duration(k) * time.Minute),
			Reading:       Reading{Unit: "Walk-in", TempC: f(3.4)},
			Outcome:       OutcomePass,
			OutcomeReason: "ok",
		}, prev)
		out = append(out, r)
		prev = &out[len(out)-1]
	}
	return out
}

func TestSealAndVerify(t *testing.T) {
	recs := chain(t, 5)
	assert.Equal(t, int64(1), recs[0].Seq)
	assert.Empty(t, recs[0].PrevHash)
	assert.Equal(t, recs[0].Hash, recs[1].PrevHash)
	assert.Len(t, recs[0].Hash, 64)

	rep := VerifyChain(recs)
	assert.True(t, rep.OK)
	assert.Equal(t, 5, rep.Checked)
	assert.Equal(t, int64(5), rep.HeadSeq)
	assert.Equal(t, recs[4].Hash, rep.HeadHash)
}

func TestVerifyFindsFirstBreak(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func([]Record) []Record
		seq     int64
		problem string
	}{
		{"edited reading", func(r []Record) []Record { r[2].Reading.TempC = f(9.9); return r }, 3, "changed"},
		{"edited outcome", func(r []Record) []Record { r[1].Outcome = OutcomeFail; return r }, 2, "changed"},
		{"deleted record", func(r []Record) []Record { return append(r[:2], r[3:]...) }, 4, "missing"},
		{"reordered", func(r []Record) []Record { r[1], r[2] = r[2], r[1]; return r }, 3, "missing"},
		{"rehashed after edit", func(r []Record) []Record {
			r[2].Reading.TempC = f(9.9)
			r[2].Hash = ComputeHash(r[2])
			return r
		}, 4, "previous-hash"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := VerifyChain(tc.mutate(chain(t, 5)))
			require.False(t, rep.OK)
			require.NotNil(t, rep.Break)
			assert.Equal(t, tc.seq, rep.Break.Seq)
			assert.Contains(t, rep.Break.Problem, tc.problem)
		})
	}
}

func TestHashIsStable(t *testing.T) {
	// Pin the hash format: changing hashContent breaks every saved chain.
	r := chain(t, 1)[0]
	assert.Equal(t, "b3782249e23f87e6ad533e93c4f1510962d81711916d1a8660e7d4edbea05727", r.Hash)
	r2 := r
	r2.Hash = "ignored"
	assert.Equal(t, r.Hash, ComputeHash(r2), "the Hash field is not hashed")
}
