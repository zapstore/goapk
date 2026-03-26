package permissions

import (
	"sort"
	"testing"
)

func TestResolve_InternetAlwaysPresent(t *testing.T) {
	perms, err := Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve(nil): %v", err)
	}
	if len(perms) != 1 || perms[0] != "android.permission.INTERNET" {
		t.Errorf("Resolve(nil) = %v, want [android.permission.INTERNET]", perms)
	}
}

func TestResolve_Camera(t *testing.T) {
	perms, err := Resolve([]string{"camera"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := map[string]bool{
		"android.permission.INTERNET": true,
		"android.permission.CAMERA":   true,
	}
	got := toSet(perms)
	for k := range want {
		if !got[k] {
			t.Errorf("missing %s", k)
		}
	}
}

func TestResolve_Geolocation(t *testing.T) {
	perms, err := Resolve([]string{"geolocation"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := map[string]bool{
		"android.permission.INTERNET":              true,
		"android.permission.ACCESS_FINE_LOCATION":   true,
		"android.permission.ACCESS_COARSE_LOCATION": true,
	}
	got := toSet(perms)
	for k := range want {
		if !got[k] {
			t.Errorf("missing %s", k)
		}
	}
}

func TestResolve_Multiple(t *testing.T) {
	perms, err := Resolve([]string{"camera", "microphone", "geolocation"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []string{
		"android.permission.INTERNET",
		"android.permission.CAMERA",
		"android.permission.RECORD_AUDIO",
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.ACCESS_COARSE_LOCATION",
	}
	got := toSet(perms)
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s in %v", w, perms)
		}
	}
}

func TestResolve_Dedup(t *testing.T) {
	perms, err := Resolve([]string{"camera", "camera"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	count := 0
	for _, p := range perms {
		if p == "android.permission.CAMERA" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("CAMERA appears %d times, want 1", count)
	}
}

func TestResolve_NoOpPermissions(t *testing.T) {
	perms, err := Resolve([]string{"persistent-storage", "clipboard-read"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(perms) != 1 {
		t.Errorf("expected only INTERNET, got %v", perms)
	}
}

func TestResolve_UnknownPermission(t *testing.T) {
	_, err := Resolve([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown permission")
	}
}

func TestKnown(t *testing.T) {
	names := Known()
	if len(names) == 0 {
		t.Fatal("Known() returned empty")
	}
	sort.Strings(names)
	if names[0] == "" {
		t.Error("Known() contains empty string")
	}
}

func toSet(ss []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range ss {
		m[s] = true
	}
	return m
}
