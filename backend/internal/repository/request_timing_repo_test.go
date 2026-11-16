package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestRequestTimingLateUsageJoin(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	r := NewRequestTimingRepository(db)
	ctx := context.Background()
	id := "fbb352f3-8e91-423d-881c-922ef7ce5698"
	mock.ExpectExec("INSERT INTO gateway_request_timing_usage").WithArgs(id, int64(0), "upstream", int64(19)).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := r.LinkUsage(ctx, service.RequestTimingLink{TraceID: id, RequestID: "upstream", APIKeyID: 19}); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(requesttiming.Trace{ID: id, RequestID: "ingress", StartedAt: time.Now(), Completeness: "partial"})
	mock.ExpectQuery(regexp.QuoteMeta(timingSelect + ` WHERE t.trace_id=$1`)).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"trace", "id", "usage_request_id"}).AddRow(payload, int64(42), "upstream"))
	trace, err := r.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if trace.UsageID == nil || *trace.UsageID != 42 || trace.UsageRequestID != "upstream" {
		t.Fatalf("link lost: %#v", trace)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
