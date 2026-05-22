package std

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jiajia556/tool-box/log"
)

func TestStdLogger_DebugAlwaysAlsoToStdout(t *testing.T) {
	l := NewStdLogger()

	cfg := log.DefaultConfig()
	cfg.Level = log.LevelDebug
	cfg.Caller = false
	cfg.Encoder = "text"
	cfg.Output = "file"
	cfg.File.Path = filepath.Join(t.TempDir(), "app.log")
	if err := l.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	defer func() { _ = l.Close() }()

	t.Run("debug writes to stdout even when output=file", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		l.Debug("hello-debug")

		_ = w.Close()
		b, _ := io.ReadAll(r)
		_ = r.Close()

		if !strings.Contains(string(b), "hello-debug") {
			t.Fatalf("expected debug log to be written to stdout; got %q", string(b))
		}
	})

	t.Run("odd fields: trailing unpaired field is printed", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		l.Debug("hello-odd", "k1", "v1", "lonely")

		_ = w.Close()
		b, _ := io.ReadAll(r)
		_ = r.Close()
		out := string(b)

		if !strings.Contains(out, "hello-odd") {
			t.Fatalf("expected log message on stdout; got %q", out)
		}
		if strings.Contains(out, "__unpaired_field") {
			t.Fatalf("did not expect reserved key to be printed; got %q", out)
		}
		if !strings.Contains(out, "lonely") {
			t.Fatalf("expected trailing unpaired field to be printed; got %q", out)
		}
	})

	t.Run("info does not write to stdout when output=file", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		l.Info("hello-info")

		_ = w.Close()
		b, _ := io.ReadAll(r)
		_ = r.Close()

		if strings.Contains(string(b), "hello-info") {
			t.Fatalf("did not expect info log to be written to stdout when output=file; got %q", string(b))
		}
	})

	t.Run("caller prefix is at beginning", func(t *testing.T) {
		cfg2 := log.DefaultConfig()
		cfg2.Level = log.LevelDebug
		cfg2.Caller = true
		cfg2.Encoder = "text"
		cfg2.Output = "stdout"
		cfg2.CallDepth = 2 // 让调用栈更稳定：getCallerInfo 的 depth 会再 +1
		if err := l.SetConfig(cfg2); err != nil {
			t.Fatalf("SetConfig: %v", err)
		}

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		l.Debug("hello-caller")

		_ = w.Close()
		b, _ := io.ReadAll(r)
		_ = r.Close()
		out := string(b)

		// 期望最前面就是 (file:line) 前缀
		re := regexp.MustCompile(`^\([^:]+:\d+\) `)
		if !re.MatchString(out) {
			t.Fatalf("expected caller prefix at beginning, got %q", out)
		}
	})

	t.Run("fields order follows call-site order", func(t *testing.T) {
		cfg3 := log.DefaultConfig()
		cfg3.Level = log.LevelDebug
		cfg3.Caller = false
		cfg3.Encoder = "text"
		cfg3.Output = "stdout"
		if err := l.SetConfig(cfg3); err != nil {
			t.Fatalf("SetConfig: %v", err)
		}

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		l.Debug("hello-order", "a", 1, "b", 2, "c", 3)

		_ = w.Close()
		bts, _ := io.ReadAll(r)
		_ = r.Close()
		out := string(bts)

		idxA := strings.Index(out, "a=1")
		idxB := strings.Index(out, "b=2")
		idxC := strings.Index(out, "c=3")
		if idxA == -1 || idxB == -1 || idxC == -1 {
			t.Fatalf("expected a=1 b=2 c=3 in output; got %q", out)
		}
		if !(idxA < idxB && idxB < idxC) {
			t.Fatalf("expected fields printed in order a,b,c; got %q", out)
		}
	})
}

