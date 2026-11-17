package install_test

import (
	"reflect"
	"testing"

	"github.com/Sebas1705/WinForge/internal/install"
)

func TestCleanCLIXML(t *testing.T) {
	var got []string
	f := install.CleanCLIXML(func(s string) { got = append(got, s) })
	for _, l := range []string{
		"#< CLIXML",
		"Scanning...",
		`<Objs Version="1.1.0.1" xmlns="x"><Obj S="progress" RefId="0"><AV>Preparing modules</AV></Obj><S S="warning">Only one restore point &amp; no more_x000D__x000A_</S><S S="Error">ignored case</S></Objs>`,
		`<Objs Version="1.1.0.1"><Obj S="progress"><AV>x</AV></Obj></Objs>`,
	} {
		f(l)
	}
	want := []string{"Scanning...", "Only one restore point & no more"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
