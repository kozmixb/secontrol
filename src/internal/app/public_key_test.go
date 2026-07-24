package app

import "testing"

func TestPublicKeyWithSeControlComment(t *testing.T) {
	got := publicKeyWithSeControlComment("ssh-ed25519 AAAAC3Nza old-comment", "Production Servers")
	want := "ssh-ed25519 AAAAC3Nza secontrol-production-servers"
	if got != want {
		t.Fatalf("publicKeyWithSeControlComment() = %q; want %q", got, want)
	}
}

func TestPublicKeyWithSeControlCommentSanitizesName(t *testing.T) {
	got := publicKeyWithSeControlComment("ssh-rsa AAAAB3Nza", "  My / Primary   Key! ")
	want := "ssh-rsa AAAAB3Nza secontrol-my-primary-key"
	if got != want {
		t.Fatalf("publicKeyWithSeControlComment() = %q; want %q", got, want)
	}
}
