package server

import "testing"

func TestAgentWorkRejectsOfficialCloudProfile(t *testing.T) {
	if _, err := Parse([]string{"--root", "/tmp/source", "--store", "store", "--token-file", "/tmp/token", "--cloud-account-file", "/tmp/account"}); err == nil {
		t.Fatal("official cloud configuration accepted")
	}
}
