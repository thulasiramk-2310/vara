package commands

import (
	"os"
	"testing"
)

// chdir switches into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestDoctorHealthyRepo(t *testing.T) {
	root, _ := makeServer(t) // a repo with one commit on main
	chdir(t, root)
	if err := RunDoctor(nil, "test", AuthConfig{}); err != nil {
		t.Fatalf("healthy repo should pass doctor, got %v", err)
	}
}

func TestDoctorOutsideRepo(t *testing.T) {
	chdir(t, t.TempDir())
	// No repo, no remote → info only, no failures.
	if err := RunDoctor(nil, "test", AuthConfig{}); err != nil {
		t.Fatalf("outside a repo doctor should not fail, got %v", err)
	}
}

func TestDoctorUnreachableRemoteFails(t *testing.T) {
	// Port 0 never accepts a connection → a reachability failure → non-nil error.
	chdir(t, t.TempDir())
	if err := RunDoctor([]string{"http://127.0.0.1:0/nope"}, "test", AuthConfig{}); err == nil {
		t.Fatal("unreachable remote should make doctor fail")
	}
}

func TestCountLooseObjectsAndHex(t *testing.T) {
	// isHex2 checks hex-ness only (length is the caller's concern).
	if !isHex2("ab") || !isHex2("ff") || isHex2("ag") || isHex2("A0") {
		t.Fatal("isHex2 misclassified")
	}
	dir := t.TempDir()
	// Two shard dirs with objects, plus a non-shard dir that must be ignored.
	for _, sh := range []string{"ab", "cd"} {
		if err := os.MkdirAll(dir+"/"+sh, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/"+sh+"/obj", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.MkdirAll(dir+"/refs", 0o755) // 4 chars → not a shard
	if n := countLooseObjects(dir); n != 2 {
		t.Fatalf("countLooseObjects = %d, want 2", n)
	}
}
