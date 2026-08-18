package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
	"github.com/samson-samson/hivemind/control-plane/internal/stats"
)

// 冒烟测试：创建事故 → 查列表 → 登记两条相同指纹 operation → 第二条被
// single-flight 去重 → stats 显示去重生效。同时验证证据账本/血缘/独立性。
func TestSmokeSingleFlight(t *testing.T) {
	svc := NewService(iam.NewMemoryStore(), 0, 0)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	// 1) 创建事故
	incBody := map[string]any{
		"ic_id":       "ic-alice",
		"symptom_set": []string{"k8s.pod.crashloop", "promql.cpu.usage.spike"},
		"source":      "alertmanager",
	}
	var inc iam.Incident
	postJSON(t, srv.URL+"/api/v1/incidents", incBody, http.StatusCreated, &inc)
	if inc.ID == "" || inc.Fingerprint == "" {
		t.Fatalf("incident missing id/fingerprint: %+v", inc)
	}
	if inc.Status != iam.IncidentOpen {
		t.Fatalf("expected default status open, got %s", inc.Status)
	}

	// 2) 查列表应包含它
	var list []iam.Incident
	getJSON(t, srv.URL+"/api/v1/incidents", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != inc.ID {
		t.Fatalf("list incidents = %d entries, want 1 containing %s", len(list), inc.ID)
	}

	// 3) 登记两条相同指纹的 operation
	now := time.Now().UTC()
	end := now.Add(-time.Minute)
	query := iam.QuerySpec{
		Target:      "ack-demo-cluster/k8s/pod/foo-7b9c",
		DataSource:  "prometheus",
		TimeWindow:  iam.TimeRange{Start: now.Add(-10 * time.Minute), End: &end},
		QueryAST:    "rate(container_cpu_usage_seconds_total[5m])",
		Filters:     []string{"namespace=default", "pod=foo-7b9c"},
		Tenant:      "acme",
		ToolVersion: "kubectl-v1.31",
	}
	opBody := map[string]any{"query": query, "source": "agent-alpha"}

	var op1 registerOperationResponse
	postJSON(t, srv.URL+"/api/v1/incidents/"+inc.ID+"/operations", opBody, http.StatusOK, &op1)
	if op1.DedupStatus != iam.DedupFresh {
		t.Fatalf("first operation should be fresh, got %s", op1.DedupStatus)
	}

	var op2 registerOperationResponse
	postJSON(t, srv.URL+"/api/v1/incidents/"+inc.ID+"/operations", opBody, http.StatusOK, &op2)
	if op2.DedupStatus != iam.DedupSingleFlight {
		t.Fatalf("second identical operation should be single-flight deduped, got %s", op2.DedupStatus)
	}
	if op2.Operation.Fingerprint != op1.Operation.Fingerprint {
		t.Fatalf("fingerprint mismatch: %s vs %s", op1.Operation.Fingerprint, op2.Operation.Fingerprint)
	}

	// 4) stats 显示去重生效
	var snap stats.Snapshot
	getJSON(t, srv.URL+"/api/v1/incidents/"+inc.ID+"/stats", http.StatusOK, &snap)
	if snap.TotalOperations != 2 {
		t.Fatalf("total operations = %d, want 2", snap.TotalOperations)
	}
	if snap.DedupedOperations != 1 {
		t.Fatalf("deduped operations = %d, want 1", snap.DedupedOperations)
	}
	if snap.DedupRate != 0.5 {
		t.Fatalf("dedup_rate = %v, want 0.5", snap.DedupRate)
	}

	// 5) 推证据，验证账本/血缘/独立性；完成后同指纹第三次登记应复用（reused）。
	evBody := map[string]any{
		"operation_id": op1.Operation.ID,
		"data_source":  "prometheus",
		"result":       "cpu usage spike 92% at 09:02, pod restart count 3",
		"conclusion":   "pod-7b9c 持续高 CPU 并重启",
		"source":       "agent-alpha",
	}
	var evEntry ledgerEntry
	postJSON(t, srv.URL+"/api/v1/incidents/"+inc.ID+"/evidence", evBody, http.StatusCreated, &evEntry)
	if evEntry.Seq == 0 || evEntry.Hash == "" {
		t.Fatalf("ledger entry missing seq/hash: %+v", evEntry)
	}
	if evEntry.Evidence.IndependenceScore != 1.0 {
		t.Fatalf("first evidence from prometheus should have independence 1.0, got %v", evEntry.Evidence.IndependenceScore)
	}
	if len(evEntry.Evidence.ProofTrace) != 1 || evEntry.Evidence.ProofTrace[0].LedgerSeq != evEntry.Seq {
		t.Fatalf("evidence proof_trace not pointing to ledger: %+v", evEntry.Evidence.ProofTrace)
	}

	var op3 registerOperationResponse
	postJSON(t, srv.URL+"/api/v1/incidents/"+inc.ID+"/operations", opBody, http.StatusOK, &op3)
	if op3.DedupStatus != iam.DedupReused {
		t.Fatalf("third identical operation should be reused, got %s", op3.DedupStatus)
	}
	if op3.ReusedEvidenceID == "" {
		t.Fatalf("reused operation should reference evidence, got empty")
	}

	// 6) 证据列表 + 账本校验
	var evList evidenceListResponse
	getJSON(t, srv.URL+"/api/v1/incidents/"+inc.ID+"/evidence", http.StatusOK, &evList)
	if len(evList.Evidence) != 1 {
		t.Fatalf("evidence list = %d, want 1", len(evList.Evidence))
	}
	if evList.TotalIndep != 1.0 {
		t.Fatalf("total independence = %v, want 1.0", evList.TotalIndep)
	}
	if ok, err := svc.ledger.Verify(context.Background()); err != nil || !ok {
		t.Fatalf("ledger hash chain invalid: ok=%v err=%v", ok, err)
	}
}

// 并发 single-flight：多个相同指纹并发登记，应只有一个是 fresh，其余合并。
func TestSmokeConcurrentMerge(t *testing.T) {
	svc := NewService(iam.NewMemoryStore(), 0, 0)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	var inc iam.Incident
	postJSON(t, srv.URL+"/api/v1/incidents", map[string]any{"symptom_set": []string{"s1"}}, http.StatusCreated, &inc)

	query := iam.QuerySpec{
		Target:     "cluster/k8s/deploy/nginx",
		DataSource: "k8s",
		QueryAST:   "kubectl get pods -A",
		Tenant:     "acme",
	}
	opBody := map[string]any{"query": query, "source": "agent-1"}
	url := srv.URL + "/api/v1/incidents/" + inc.ID + "/operations"

	const n = 8
	var wg sync.WaitGroup
	resCh := make(chan registerOperationResponse, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := doJSON(http.MethodPost, url, opBody)
			if err != nil {
				t.Errorf("concurrent request error: %v", err)
				return
			}
			resCh <- res
		}()
	}
	wg.Wait()
	close(resCh)

	fresh, merged := 0, 0
	for res := range resCh {
		switch res.DedupStatus {
		case iam.DedupFresh:
			fresh++
		case iam.DedupSingleFlight:
			merged++
		default:
			t.Fatalf("unexpected status %s", res.DedupStatus)
		}
	}
	if fresh != 1 {
		t.Fatalf("concurrent identical queries: fresh = %d, want exactly 1", fresh)
	}
	if merged != n-1 {
		t.Fatalf("concurrent identical queries: merged = %d, want %d", merged, n-1)
	}
}

// ---- 辅助 ----

type ledgerEntry struct {
	Seq      int64         `json:"seq"`
	Evidence *iam.Evidence `json:"evidence"`
	Hash     string        `json:"hash"`
}

func postJSON(t *testing.T, url string, body any, wantStatus int, dst any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "hivemind-dev-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func getJSON(t *testing.T, url string, wantStatus int, dst any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Key", "hivemind-dev-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func doJSON(method, url string, body any) (registerOperationResponse, error) {
	var res registerOperationResponse
	b, err := json.Marshal(body)
	if err != nil {
		return res, err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(b))
	if err != nil {
		return res, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "hivemind-dev-key") // 认证（安全基线）
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("status %d", httpResp.StatusCode)
	}
	err = json.NewDecoder(httpResp.Body).Decode(&res)
	return res, err
}

// TestSymptomJaccard 召回相似度（防回归）：
// LLM 蒸馏措辞 vs 告警原始措辞，差一字/多一字都要有强信号。
func TestSymptomJaccard(t *testing.T) {
	cases := []struct {
		a, b string
		want float64 // 期望下限（>=want 即通过）
	}{
		// 同语义不同措辞：浓缩 vs 原始（原始召回场景）
		{"GPU利用率异常", "GPU 利用率低于阈值", 0.2},
		{"请求超时取消", "请求超时被取消", 0.4},
		// 完全无关
		{"GPU利用率异常", "casdoor认证延迟", 0.0},
		// 英文整词
		{"KV cache 耗尽", "kv cache", 0.5},
	}
	for _, c := range cases {
		got := symptomJaccard(c.a, c.b)
		if got < c.want {
			t.Errorf("symptomJaccard(%q, %q)=%.3f, want >=%.2f", c.a, c.b, got, c.want)
		} else {
			t.Logf("symptomJaccard(%q, %q)=%.3f ✓", c.a, c.b, got)
		}
	}
}

// TestKnowledgeRecall E2E：浓缩 runbook 认证后，措辞不同的相似事故能召回。
func TestKnowledgeRecall(t *testing.T) {
	svc := NewService(iam.NewMemoryStore(), 0, 0)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	// 事故 A + 浓缩症状 runbook（模拟 distiller 产物）→ certified
	var incA iam.Incident
	postJSON(t, srv.URL+"/api/v1/incidents", map[string]any{
		"title": "gpu-utilization-prod", "severity": "P1", "ic_id": "ic-alice",
		"symptom_set": []string{"GPU利用率异常", "请求超时取消"}, "source": "alertmanager",
	}, http.StatusCreated, &incA)

	var rb iam.Runbook
	postJSON(t, srv.URL+"/api/v1/incidents/"+incA.ID+"/runbooks", map[string]any{
		"title": "GPU利用率异常且请求超时取消", "root_cause": "GPU调度层异常/KV cache耗尽",
		"symptoms": []string{"GPU利用率异常", "请求超时取消", "推理服务排队"},
		"diagnostic_steps": []string{"查调度日志", "查KV cache指标"},
		"verification_actions": []string{"扩容推理Pod"}, "rollback": "恢复原配置",
		"success_criteria": "GPU利用率恢复基线",
	}, http.StatusCreated, &rb)

	// IC 认证（certify 需要认证用户；内置 dev key → zhangqian）
	req := httptest.NewRequest("PATCH", "/api/v1/runbooks/"+rb.ID, bytes.NewBufferString(`{"status":"certified"}`))
	req.Header.Set("X-API-Key", "hivemind-dev-key")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("certify status=%d body=%s", rr.Code, rr.Body.String())
	}

	// 事故 B：措辞不同但语义相同 → 应召回 certified runbook
	var incB iam.Incident
	postJSON(t, srv.URL+"/api/v1/incidents", map[string]any{
		"title": "gpu-utilization-low-prod", "severity": "P2", "ic_id": "ic-alice",
		"symptom_set": []string{"GPU 利用率低于阈值", "请求超时被取消", "KV cache"}, "source": "alertmanager",
	}, http.StatusCreated, &incB)

	var hits []knowledgeHit
	getJSON(t, srv.URL+"/api/v1/incidents/"+incB.ID+"/knowledge", http.StatusOK, &hits)
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d: %+v", len(hits), hits)
	}
	if !hits[0].Certified {
		t.Errorf("certified runbook should rank first: %+v", hits[0])
	}
	if hits[0].Score < 0.3 {
		t.Errorf("score=%.3f, want >=0.3 (浓缩vs原始措辞要有强信号)", hits[0].Score)
	}
	t.Logf("recall hit: score=%.3f certified=%v %s", hits[0].Score, hits[0].Certified, hits[0].Title)
}
