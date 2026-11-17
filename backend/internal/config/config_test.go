package config

import "testing"

func TestRatesFinite(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "-1", "oops"} {
		if _, err := rate(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	if v, err := rate(""); err != nil || v != nil {
		t.Fatal("missing rate must stay unknown")
	}
	if v, err := rate("0.075"); err != nil || *v != 0.075 {
		t.Fatal(v, err)
	}
}
