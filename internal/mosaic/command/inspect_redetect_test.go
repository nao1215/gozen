package command

import "testing"

// TestInspectRedetectKeepsAMatchingReportDone: --redetect runs the model again instead of
// trusting report.json, but when the output still covers what is found the picture is done;
// the report is not out of date just because it was checked afresh.
func TestInspectRedetectKeepsAMatchingReportDone(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	base := []string{"--model", model, "-o", out, game, "pattern.png"}
	if code, _, diag := invoke(t, append([]string{"auto"}, base...)...); code != 0 {
		t.Fatal(diag)
	}
	before := det.calls
	in := inspectOne(t, append([]string{"--redetect"}, base...)...)
	if det.calls != before+1 {
		t.Fatalf("--redetect did not detect again (%d calls)", det.calls-before)
	}
	if in.Status != "done" || !in.Mosaicked.OutputCovers {
		t.Fatalf("status %s, output covers %v, note %q", in.Status, in.Mosaicked.OutputCovers, in.Mosaicked.Note)
	}
	// What the model finds now that the output does not cover is still to do.
	det.boxes = append(det.boxes, penis(14, 14, 20, 20, 0.9))
	in = inspectOne(t, append([]string{"--redetect"}, base...)...)
	if in.Status != "todo" {
		t.Fatalf("a new detection outside the output: status %s", in.Status)
	}
}
