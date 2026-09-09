package provider

import "testing"

func TestPickCheapestHealthy(t *testing.T) {
	cands := []Candidate{
		{Provider: "paid", Model: "sonnet", CostPer1M: 3, Healthy: true, QuotaLeft: 100},
		{Provider: "free-pool", Model: "sonnet-free", CostPer1M: 0, Healthy: true, QuotaLeft: 10},
		{Provider: "dead", Model: "x", CostPer1M: 0, Healthy: false, QuotaLeft: 99},
		{Provider: "empty", Model: "y", CostPer1M: 0, Healthy: true, QuotaLeft: 0},
	}
	got := Pick(cands)
	if got == nil || got.Provider != "free-pool" {
		t.Fatalf("debio elegir free-pool sano con quota, got %+v", got)
	}
}

func TestDialectKimi(t *testing.T) {
	if DialectFor("kimi-k2") != "kimi_k2_vllm" {
		t.Fatalf("kimi-k2 debe usar vllm")
	}
	if DialectFor("kimi-k2.5") == "kimi_k2_vllm" {
		t.Fatalf("k2.5 no debe usar vllm (bug #102)")
	}
	if DialectFor("kimi-k2-6") == "kimi_k2_vllm" {
		t.Fatalf("k2.6 no debe usar vllm (bug #102)")
	}
}
