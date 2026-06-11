package config

import "testing"

func TestEnvFloatReadsDecimalValues(t *testing.T) {
	t.Setenv("POST_TEST_FLOAT", " 3.5 ")

	got := Env{}.Float("POST_TEST_FLOAT", 1.25)

	if got != 3.5 {
		t.Fatalf("expected parsed float 3.5, got %v", got)
	}
}

func TestEnvFloatFallsBackForInvalidValues(t *testing.T) {
	tests := []string{"", "invalid", "NaN", "+Inf"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("POST_TEST_FLOAT", value)

			got := Env{}.Float("POST_TEST_FLOAT", 1.25)

			if got != 1.25 {
				t.Fatalf("expected default float 1.25 for %q, got %v", value, got)
			}
		})
	}
}

func TestEnvBoolNormalizesStringFlags(t *testing.T) {
	tests := map[string]bool{
		" TRUE ": true,
		"yes":    true,
		"On":     true,
		"0":      false,
		"FALSE":  false,
		" off ":  false,
	}

	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("POST_TEST_BOOL", value)

			got := Env{}.Bool("POST_TEST_BOOL", !expected)

			if got != expected {
				t.Fatalf("expected bool %v for %q, got %v", expected, value, got)
			}
		})
	}
}
