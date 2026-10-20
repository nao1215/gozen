package command

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func setupWith(t *testing.T, args ...string) (config, error) {
	t.Helper()
	game, out := fixture(t)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	o := (&cli{}).commonFlags(fs)
	if err := fs.Parse(append(args, "-o", out)); err != nil {
		t.Fatal(err)
	}
	return o.setup(game)
}

func labelSet(c config) []string {
	var out []string
	for l := range c.p.labels {
		out = append(out, l)
	}
	slices.Sort(out)
	return out
}

func TestClassesDefaultKeepsBehavior(t *testing.T) {
	c, err := setupWith(t)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"nipple_f", "penis", "pussy"}; !slices.Equal(c.classes, want) {
		t.Fatalf("classes = %v, want %v", c.classes, want)
	}
	if got, want := labelSet(c), []string{"penis", "pussy"}; !slices.Equal(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestClassesReplaceOrderAndDefaultLabels(t *testing.T) {
	c, err := setupWith(t, "--classes", "face, hand")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"face", "hand"}; !slices.Equal(c.classes, want) {
		t.Fatalf("classes = %v", c.classes)
	}
	if got := labelSet(c); !slices.Equal(got, []string{"face", "hand"}) {
		t.Fatalf("without --labels every class is a target, got %v", got)
	}
	c, err = setupWith(t, "--classes", "face,hand", "--labels", "hand")
	if err != nil {
		t.Fatal(err)
	}
	if got := labelSet(c); !slices.Equal(got, []string{"hand"}) {
		t.Fatalf("labels = %v", got)
	}
}

func TestClassesInvalid(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--classes", "face,,hand"}, "empty class name"},
		{[]string{"--classes", ",face"}, "empty class name"},
		{[]string{"--classes", "face,face"}, "twice"},
		{[]string{"--classes", "face", "--labels", "hand"}, "unknown label"},
		{[]string{"--labels", "face"}, "unknown label"},
	} {
		_, err := setupWith(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}
