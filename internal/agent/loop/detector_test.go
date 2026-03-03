package loop

import "testing"

func TestDetectorNoLoop(t *testing.T) {
	d := NewDetector(10)
	if d.Record("read_file", `{"path":"a.go"}`) {
		t.Error("no loop on first call")
	}
	if d.Record("write_file", `{"path":"b.go"}`) {
		t.Error("no loop on different calls")
	}
}

func TestDetectorPeriod1(t *testing.T) {
	d := NewDetector(10)
	d.Record("shell", `{"cmd":"ls"}`)
	d.Record("shell", `{"cmd":"ls"}`)
	if !d.Record("shell", `{"cmd":"ls"}`) {
		t.Error("should detect period-1 loop (AAA)")
	}
}

func TestDetectorPeriod2(t *testing.T) {
	d := NewDetector(10)
	d.Record("read_file", `{"path":"a.go"}`)
	d.Record("write_file", `{"path":"a.go"}`)
	d.Record("read_file", `{"path":"a.go"}`)
	if !d.Record("write_file", `{"path":"a.go"}`) {
		t.Error("should detect period-2 loop (ABAB)")
	}
}

func TestDetectorPeriod3(t *testing.T) {
	d := NewDetector(10)
	d.Record("read_file", `{"path":"a.go"}`)
	d.Record("edit_file", `{"path":"a.go"}`)
	d.Record("shell", `{"cmd":"go test"}`)
	d.Record("read_file", `{"path":"a.go"}`)
	d.Record("edit_file", `{"path":"a.go"}`)
	if !d.Record("shell", `{"cmd":"go test"}`) {
		t.Error("should detect period-3 loop (ABCABC)")
	}
}

func TestDetectorReset(t *testing.T) {
	d := NewDetector(10)
	d.Record("shell", `{"cmd":"ls"}`)
	d.Record("shell", `{"cmd":"ls"}`)
	d.Reset()
	if d.Record("shell", `{"cmd":"ls"}`) {
		t.Error("should not detect loop after reset")
	}
}

func TestDetectorWindowTrim(t *testing.T) {
	d := NewDetector(5)
	// Fill with unique calls.
	for i := 0; i < 10; i++ {
		d.Record("shell", string(rune('a'+i)))
	}
	// Window should be trimmed to 5.
	if len(d.signatures) > 5 {
		t.Errorf("signatures = %d, want <= 5", len(d.signatures))
	}
}

func TestDetectorDefaultWindow(t *testing.T) {
	d := NewDetector(0)
	if d.window != 10 {
		t.Errorf("default window = %d, want 10", d.window)
	}
}
