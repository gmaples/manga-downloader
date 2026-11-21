// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "fmt"

// The ages the age check offers: the user is over 16, or over 18
const (
	AgeTeen  = 16
	AgeAdult = 18
)

// NormalizeAge returns age when it's one the check offers, AgeAdult otherwise
// (a settings file from before the check existed, or a hand edit)
func NormalizeAge(age int) int {
	if age == AgeTeen {
		return AgeTeen
	}
	return AgeAdult
}

// AgeAllows returns an error when the age check is on and the site rates the
// title above the user's age. rating is the site's minimum age for the title,
// 0 when it has none. A grabber whose site rates titles calls it once it has
// read the rating, so the title is refused before anything is downloaded.
func (s *Settings) AgeAllows(rating int) error {
	if s == nil || !s.AgeCheck || rating <= 0 {
		return nil
	}
	if age := NormalizeAge(s.Age); rating > age {
		return fmt.Errorf("the site rates this title %d+, above the age check's %d (turn the age check off or raise it to load it)", rating, age)
	}
	return nil
}
