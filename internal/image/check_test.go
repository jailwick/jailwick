package image

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func (d *fakeDriver) RemoveImage(context.Context, string) error { return nil }

func TestCheckReadsTheFindings(t *testing.T) {
	d := &fakeDriver{output: "#5 [3/3] RUN sh /tmp/jwcheck.sh\n" +
		"#5 0.21 JWCHECK ok distro Debian GNU/Linux 12 (bookworm)\n" +
		"#5 0.22 JWCHECK bad user no user app: set the sandbox user to one the image has\n"}
	b := &Builder{Driver: d, StateDir: t.TempDir()}
	got, err := b.Check(context.Background(), CheckOptions{Ref: "ruby:3.3-bookworm", User: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].OK || got[0].Detail != "Debian GNU/Linux 12 (bookworm)" || got[1].OK || got[1].Name != "user" {
		t.Fatalf("%+v", got)
	}
	o := d.built[0]
	if !o.NoCache || !strings.HasPrefix(o.Tag, "jailwick-check:") {
		t.Fatalf("built %+v", o)
	}
}

func TestCheckSaysWhyItCouldNot(t *testing.T) {
	for out, want := range map[string]string{
		"ERROR: failed to resolve source metadata for docker.io/library/ruby:9: not found": "doesn't exist",
		"pull access denied, repository does not exist or may require authorization":       "doesn't exist",
		"no match for platform in manifest":                                                "architecture",
		`exec: "/bin/sh": stat /bin/sh: no such file or directory`:                         "no shell",
	} {
		d := &fakeDriver{output: out, fail: errors.New("exit 1")}
		_, err := (&Builder{Driver: d, StateDir: t.TempDir()}).Check(context.Background(), CheckOptions{Ref: "x"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", out, err)
		}
	}
}
