package core

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSizeText(t *testing.T) {
	for n, want := range map[int64]string{
		0:                 "0 B",
		999:               "999 B",
		1000:              "1.0 kB",
		912_400:           "912.4 kB",
		12_300_000:        "12.3 MB",
		6_400_000_000:     "6.4 GB",
		1_000_000_000_000: "1.0 TB",
	} {
		if got := SizeText(n); got != want {
			t.Errorf("SizeText(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBytesPrintsAsTextAndTravelsAsANumber(t *testing.T) {
	b := Bytes(6_400_000_000)
	if got := fmt.Sprint(b); got != "6.4 GB" {
		t.Errorf("fmt.Sprint = %q, want 6.4 GB", got)
	}
	data, err := json.Marshal(map[string]any{"size": b})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `{"size":6400000000}` {
		t.Errorf("json = %s, want the bare number", got)
	}
}
