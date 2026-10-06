package health

import (
	"strings"
	"testing"
	"time"
)

func TestStepWriterReportsStagesAndKeepsOutput(t *testing.T) {
	var got []string
	w := &stepWriter{onStep: func(s string) { got = append(got, s) }}
	// Chunks split mid-line, as a pipe may deliver them.
	for _, c := range []string{"##STEP sys", "tem\r\n##STEP drivers\n", `{"admin":true}`} {
		_, _ = w.Write([]byte(c))
	}
	if strings.Join(got, ",") != "system,drivers" {
		t.Fatalf("steps = %v", got)
	}
	r, err := ParseReport(w.buf.Bytes(), time.Now())
	if err != nil || !r.Admin {
		t.Fatalf("report not parsed after step lines: %v", err)
	}
}

func TestEveryCollectStepIsEmittedByTheScript(t *testing.T) {
	for _, s := range CollectSteps {
		if !strings.Contains(collectScript, "Step '"+s+"'") {
			t.Errorf("collectScript never reports step %q", s)
		}
	}
	if n := strings.Count(collectScript, "\nStep '"); n != len(CollectSteps) {
		t.Errorf("script has %d steps, CollectSteps lists %d", n, len(CollectSteps))
	}
}
