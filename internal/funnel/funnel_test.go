package funnel

import (
	"reflect"
	"testing"
)

func TestBuildStartArgs(t *testing.T) {
	got := BuildStartArgs("/hooks", "http://127.0.0.1:8080/.ft/hooks")
	want := []string{"funnel", "--bg", "--yes", "--set-path", "/hooks", "http://127.0.0.1:8080/.ft/hooks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBuildStopArgs(t *testing.T) {
	got := BuildStopArgs("/hooks")
	want := []string{"funnel", "--yes", "--set-path", "/hooks", "off"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
