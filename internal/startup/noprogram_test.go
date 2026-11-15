package startup

import "testing"

func TestListValueThatIsNoProgram(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "AF_counter", `10`)
	f.addRun(UserRun, "AF_uuid", `e3739610-e6c2-4370-846b-c6992c6fa761`)
	items, _ := mgr(f, false).List()
	if len(items) != 2 {
		t.Fatalf("%+v", items)
	}
	for _, it := range items {
		if it.Target != "" || it.Missing || it.Publisher != "" {
			t.Errorf("a value that is no program has no target to show or to miss: %+v", it)
		}
	}
}
