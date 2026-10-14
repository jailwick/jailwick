package config

import (
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	for name, ok := range map[string]bool{
		"api": true, "my-app": true, "a1": true, strings.Repeat("a", 40): true,
		"": false, "-api": false, "My": false, "a_b": false, "a--b": false, // -- separates sessions
		strings.Repeat("a", 41): false,
	} {
		if err := ValidName(name); (err == nil) != ok {
			t.Errorf("ValidName(%q) = %v", name, err)
		}
	}
}
