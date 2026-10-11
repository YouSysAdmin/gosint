// Gosint, Copyright (c) 2026 YouSysAdmin

package envelope

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"testing"
)

// A sealed file opens with its passphrase only, is told from a plain
// one, and a changed byte is refused.
func TestSealAndOpen(t *testing.T) {
	plain := []byte(`{"gosint":"sketch","nodes":[]}`)
	sealed, err := Seal("correct horse", plain)
	if err != nil {
		t.Fatal(err)
	}

	if !IsSealed(sealed) || IsSealed(plain) {
		t.Fatal("IsSealed does not tell the two apart")
	}

	if bytes.Contains(sealed, []byte("sketch")) {
		t.Fatal("the plaintext shows through")
	}

	out, err := Open("correct horse", sealed)
	if err != nil || !bytes.Equal(out, plain) {
		t.Fatalf("open: %v %q", err, out)
	}

	if _, err := Open("wrong", sealed); !errors.Is(err, ErrPassphrase) {
		t.Errorf("wrong passphrase: %v", err)
	}

	if _, err := Open("correct horse", plain); !errors.Is(err, ErrNotSealed) {
		t.Errorf("a plain file: %v", err)
	}

	changed := bytes.Replace(sealed, []byte(`"data":"`), []byte(`"data":"AA`), 1)
	if _, err := Open("correct horse", changed); err == nil {
		t.Error("a changed file opened")
	}

	if _, err := Seal(" ", plain); err == nil {
		t.Error("an empty passphrase sealed")
	}
}

// A file asking for more work than Seal ever writes is refused before
// any hashing.
func TestCostsAboveTheWritersAreRefused(t *testing.T) {
	sealed, err := Seal("correct horse", []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}

	var f file
	if err := json.Unmarshal(sealed, &f); err != nil {
		t.Fatal(err)
	}

	for name, tune := range map[string]func(*file){
		"memory":  func(f *file) { f.Memory = argonMemory * 4 },
		"passes":  func(f *file) { f.Time = argonTime + 1 },
		"threads": func(f *file) { f.Threads = argonThreads + 1 },
		"salt":    func(f *file) { f.Salt = make([]byte, 1<<20) },
	} {
		g := f
		tune(&g)
		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := Open("correct horse", raw); !errors.Is(err, ErrNotSealed) {
			t.Errorf("%s above the writer's: %v, want ErrNotSealed", name, err)
		}
	}
}
