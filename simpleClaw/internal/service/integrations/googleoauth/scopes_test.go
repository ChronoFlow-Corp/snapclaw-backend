package googleoauth

import (
	"reflect"
	"testing"
)

func TestNormalizeCapabilities(t *testing.T) {
	got, err := NormalizeCapabilities([]string{
		" gmail ",
		"google_calendar",
		"GMAIL",
		"sheets",
		"google_calendar",
	})
	if err != nil {
		t.Fatalf("NormalizeCapabilities() error = %v", err)
	}

	want := []string{"gmail", "google_calendar", "sheets"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCapabilities() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCapabilitiesRejectsUnsupported(t *testing.T) {
	if _, err := NormalizeCapabilities([]string{"gmail", "github"}); err == nil {
		t.Fatal("NormalizeCapabilities() error = nil, want error")
	}
}

func TestBuildScopes(t *testing.T) {
	got, err := BuildScopes([]string{
		"google_calendar",
		"gmail",
		"gmail",
	})
	if err != nil {
		t.Fatalf("BuildScopes() error = %v", err)
	}

	want := []string{
		"openid",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/calendar",
		"https://www.googleapis.com/auth/gmail.modify",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildScopes() = %#v, want %#v", got, want)
	}
}

func TestCapabilitiesFromGrantedScopes(t *testing.T) {
	got := CapabilitiesFromGrantedScopes([]string{
		"https://www.googleapis.com/auth/spreadsheets",
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/userinfo.email",
	})

	want := []string{"gmail", "sheets"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CapabilitiesFromGrantedScopes() = %#v, want %#v", got, want)
	}
}
