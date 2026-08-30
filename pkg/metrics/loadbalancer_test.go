package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordLoadBalancer_OneSeriesPerService(t *testing.T) {
	t.Cleanup(func() { DeleteLoadBalancer("ns", "web") })
	RecordLoadBalancer("ns", "web", "", "", "pending")
	RecordLoadBalancer("ns", "web", "prod", "sg-01", "bound")
	if n := testutil.CollectAndCount(lbService); n != 1 {
		t.Fatalf("series = %d, want 1", n)
	}
	if v := testutil.ToFloat64(lbService.WithLabelValues("ns", "web", "prod", "sg-01", "bound")); v != 1 {
		t.Fatalf("bound = %v", v)
	}
	DeleteLoadBalancer("ns", "web")
	if n := testutil.CollectAndCount(lbService); n != 0 {
		t.Fatalf("after delete series = %d", n)
	}
}

func TestPoolAllocated(t *testing.T) {
	t.Cleanup(func() { DeletePool("prod") })
	SetPoolAllocated("prod", "sg-01", 2)
	SetPoolAllocated("prod", "sg-02", 0)
	if n := testutil.CollectAndCount(poolAllocated); n != 2 {
		t.Fatalf("series = %d", n)
	}
	if v := testutil.ToFloat64(poolAllocated.WithLabelValues("prod", "sg-01")); v != 2 {
		t.Fatalf("sg-01 = %v", v)
	}
	DeletePool("prod")
	if n := testutil.CollectAndCount(poolAllocated); n != 0 {
		t.Fatalf("after delete series = %d", n)
	}
}
