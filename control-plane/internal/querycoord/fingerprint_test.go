package querycoord

import (
	"testing"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

func baseSpec() iam.QuerySpec {
	start := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 12, 9, 5, 30, 123456789, time.UTC)
	return iam.QuerySpec{
		Target:      "ack-demo-cluster/k8s/pod/foo-7b9c",
		DataSource:  "Prometheus",
		TimeWindow:  iam.TimeRange{Start: start, End: &end},
		QueryAST:    "rate(container_cpu_usage_seconds_total[5m])",
		Filters:     []string{"namespace=default", "pod=foo-7b9c"},
		Tenant:      "ACME",
		ToolVersion: "kubectl-v1.31",
	}
}

func TestFingerprintCanonicalization(t *testing.T) {
	fp1, err := Fingerprint(baseSpec())
	if err != nil {
		t.Fatal(err)
	}

	// 字段顺序、大小写、空白差异不应改变指纹（完全相同查询）。
	base := baseSpec()
	permuted := iam.QuerySpec{
		Target:      "  " + base.Target + "  ",
		DataSource:  "prometheus", // 小写化
		QueryAST:    "  rate(container_cpu_usage_seconds_total[5m])  ",
		Filters:     []string{"pod=foo-7b9c", "namespace=default"}, // 乱序
		Tenant:      "acme",                                        // 小写化
		ToolVersion: base.ToolVersion,
		TimeWindow:  base.TimeWindow,
	}
	fp2, err := Fingerprint(permuted)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("canonicalization failed:\n%s\n%s", fp1, fp2)
	}
}

func TestFingerprintDiffersForDifferentQuery(t *testing.T) {
	fp1, _ := Fingerprint(baseSpec())
	other := baseSpec()
	other.QueryAST = "rate(container_memory_usage_bytes[5m])"
	fp2, _ := Fingerprint(other)
	if fp1 == fp2 {
		t.Fatal("different query must produce different fingerprint")
	}
}
