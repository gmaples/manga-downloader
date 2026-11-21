// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestAgeAllows(t *testing.T) {
	cases := []struct {
		check  bool
		age    int
		rating int
		ok     bool
	}{
		{false, 16, 18, true}, // off: every title loads
		{true, 16, 0, true},   // no rating
		{true, 16, 16, true},
		{true, 16, 18, false},
		{true, 18, 18, true},
		{true, 0, 18, true}, // an unset age is 18
		{true, 0, 21, false},
	}
	for _, c := range cases {
		s := &Settings{AgeCheck: c.check, Age: c.age}
		if err := s.AgeAllows(c.rating); (err == nil) != c.ok {
			t.Errorf("AgeAllows(%d) with check %v, age %d = %v, want ok %v", c.rating, c.check, c.age, err, c.ok)
		}
	}
	var none *Settings
	if err := none.AgeAllows(18); err != nil {
		t.Errorf("nil settings: %v", err)
	}
}
