package identity

import (
	"os"
	"sync"
	"testing"
)

func TestAtomicIdentitySave(t *testing.T) {
	s := NewStore(t.TempDir())
	id := &Identity{NodeID: "n", Secret: "secret"}
	if err := s.Save(id); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if err := s.Save(id); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := 0; i < 100; i++ {
		loaded, err := s.Load()
		if err != nil || loaded == nil || loaded.Secret != id.Secret {
			t.Errorf("concurrent read saw partial identity: %v", err)
			break
		}
	}
	wg.Wait()
	st, err := os.Stat(s.Path())
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("identity is not private")
	}
}
