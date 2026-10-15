package main

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeDSN(t *testing.T) {
	got, err := normalizeDSN("app:p@ss;w0rd@unix(/run/mysqld/mysqld.sock)/movies")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"parseTime=true", "time_zone=%27%2B00%3A00%27", "unix(/run/mysqld/mysqld.sock)"} {
		if !strings.Contains(got, want) {
			t.Errorf("normalizeDSN() = %q, missing %q", got, want)
		}
	}

	// An explicit time_zone from the operator is kept.
	got, err = normalizeDSN("u:p@tcp(db:3306)/movies?time_zone=%27%2B07%3A00%27")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "time_zone=%27%2B07%3A00%27") {
		t.Errorf("explicit time_zone overwritten: %q", got)
	}

	if _, err := normalizeDSN("not a dsn"); err == nil {
		t.Error("invalid DSN should fail")
	}
}

func TestHumanDateUsesLocalZone(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })
	time.Local = time.FixedZone("ICT", 7*60*60)

	utc := time.Date(2026, 10, 15, 17, 30, 0, 0, time.UTC)
	if got := humanDate(utc); got != "16 Oct 2026 at 00:30" {
		t.Errorf("humanDate() = %q", got)
	}
	if humanDate(time.Time{}) != "" {
		t.Error("zero time should render empty")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("MR_TEST_BOOL", "false")
	t.Setenv("MR_TEST_BAD", "maybe")
	t.Setenv("MR_TEST_STR", "value")
	if envBool("MR_TEST_BOOL", true) {
		t.Error("envBool should parse false")
	}
	if !envBool("MR_TEST_BAD", true) {
		t.Error("invalid bool should use fallback")
	}
	if envString("MR_TEST_STR", "x") != "value" || envString("MR_TEST_MISSING", "x") != "x" {
		t.Error("envString")
	}
}
