package iam

import "testing"

func TestIncidentFingerprintOrderInsensitive(t *testing.T) {
	fp1 := IncidentFingerprint([]string{"k8s.pod.crashloop", "cpu.spike"})
	fp2 := IncidentFingerprint([]string{"cpu.spike", "k8s.pod.crashloop"})
	if fp1 != fp2 {
		t.Fatalf("fingerprint should be order-insensitive:\n%s\n%s", fp1, fp2)
	}
}

func TestIncidentFingerprintDistinct(t *testing.T) {
	fp1 := IncidentFingerprint([]string{"a"})
	fp2 := IncidentFingerprint([]string{"b"})
	if fp1 == fp2 {
		t.Fatal("distinct symptom sets must produce distinct fingerprints")
	}
}
