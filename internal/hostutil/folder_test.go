package hostutil

import "testing"

func TestAppleString(t *testing.T) {
	for in, want := range map[string]string{
		`/Users/me/code`:      `"/Users/me/code"`,
		`/Users/me/"odd" dir`: `"/Users/me/\"odd\" dir"`,
		`C:\dir`:              `"C:\\dir"`,
	} {
		if got := appleString(in); got != want {
			t.Errorf("appleString(%q) = %s, want %s", in, got, want)
		}
	}
}
