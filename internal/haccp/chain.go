package haccp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// hashVersion is bumped if the hashed content ever changes shape, so old
// records still verify under the rules they were written with.
const hashVersion = 1

// hashContent is exactly what a record hash covers, in a fixed order.
type hashContent struct {
	V                int        `json:"v"`
	ID               string     `json:"id"`
	Seq              int64      `json:"seq"`
	Type             RecordType `json:"type"`
	LoggedBy         string     `json:"logged_by"`
	LoggedByName     string     `json:"logged_by_name"`
	LoggedAt         string     `json:"logged_at"`
	Reading          Reading    `json:"reading"`
	Outcome          Outcome    `json:"outcome"`
	OutcomeReason    string     `json:"outcome_reason"`
	LimitVersionID   string     `json:"limit_version_id"`
	LimitScope       *Scope     `json:"limit_scope"`
	CorrectiveAction string     `json:"corrective_action"`
	Corrects         string     `json:"corrects"`
	CorrectionReason string     `json:"correction_reason"`
	PrevHash         string     `json:"prev_hash"`
}

// ComputeHash returns the SHA-256 of a record's content and PrevHash, as
// lower-case hex. The Hash field itself is not included.
func ComputeHash(r Record) string {
	b, err := json.Marshal(hashContent{
		V:                hashVersion,
		ID:               r.ID,
		Seq:              r.Seq,
		Type:             r.Type,
		LoggedBy:         r.LoggedBy,
		LoggedByName:     r.LoggedByName,
		LoggedAt:         r.LoggedAt.UTC().Format(time.RFC3339Nano),
		Reading:          r.Reading,
		Outcome:          r.Outcome,
		OutcomeReason:    r.OutcomeReason,
		LimitVersionID:   r.LimitVersionID,
		LimitScope:       r.LimitScope,
		CorrectiveAction: r.CorrectiveAction,
		Corrects:         r.Corrects,
		CorrectionReason: r.CorrectionReason,
		PrevHash:         r.PrevHash,
	})
	if err != nil {
		// Every field is a plain value; Marshal cannot fail on this struct.
		panic(fmt.Sprintf("marshal record hash content: %v", err))
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Seal sets r's Seq and PrevHash to follow prev (nil for the first record)
// and fills in Hash.
func Seal(r Record, prev *Record) Record {
	if prev == nil {
		r.Seq, r.PrevHash = 1, ""
	} else {
		r.Seq, r.PrevHash = prev.Seq+1, prev.Hash
	}
	r.Hash = ComputeHash(r)
	return r
}

// ChainBreak describes the first place a chain fails to verify.
type ChainBreak struct {
	Seq      int64  `json:"seq"`
	RecordID string `json:"record_id,omitempty"`
	Problem  string `json:"problem"`
}

// ChainReport is the result of verifying a chain.
type ChainReport struct {
	OK       bool        `json:"ok"`
	Checked  int         `json:"checked"`
	HeadSeq  int64       `json:"head_seq"`
	HeadHash string      `json:"head_hash"`
	Break    *ChainBreak `json:"break,omitempty"`
}

// ChainVerifier checks records one at a time, in sequence order, so a long
// chain can be streamed instead of loaded at once.
type ChainVerifier struct {
	report ChainReport
	prev   *Record
}

// Add checks the next record. It returns false once a break is found;
// later calls do nothing.
func (v *ChainVerifier) Add(r Record) bool {
	if v.report.Break != nil {
		return false
	}
	fail := func(problem string) bool {
		v.report.Break = &ChainBreak{Seq: r.Seq, RecordID: r.ID, Problem: problem}
		return false
	}
	wantSeq, wantPrev := int64(1), ""
	if v.prev != nil {
		wantSeq, wantPrev = v.prev.Seq+1, v.prev.Hash
	}
	switch {
	case r.Seq != wantSeq:
		return fail(fmt.Sprintf("expected record %d, found %d: records are missing or out of order", wantSeq, r.Seq))
	case r.PrevHash != wantPrev:
		return fail("previous-hash does not match the record before it")
	case ComputeHash(r) != r.Hash:
		return fail("content does not match its hash: the record was changed after it was saved")
	}
	v.report.Checked++
	v.report.HeadSeq, v.report.HeadHash = r.Seq, r.Hash
	rr := r
	v.prev = &rr
	return true
}

// Report returns the result so far.
func (v *ChainVerifier) Report() ChainReport {
	out := v.report
	out.OK = out.Break == nil
	return out
}

// VerifyChain checks a whole chain given in sequence order.
func VerifyChain(records []Record) ChainReport {
	var v ChainVerifier
	for _, r := range records {
		if !v.Add(r) {
			break
		}
	}
	return v.Report()
}
