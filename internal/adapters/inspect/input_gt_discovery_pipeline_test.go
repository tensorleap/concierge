package inspect

import (
	"reflect"
	"testing"
)

func TestExtractInputGTCandidatesFromLineIgnoresGenericLoopIdentifiers(t *testing.T) {
	line := "for x in input if isinstance(input, list) else [input]:"
	inputs, groundTruths := extractInputGTCandidatesFromLine(line)
	if len(inputs) != 0 {
		t.Fatalf("expected no input candidates for generic iterator line, got %+v", inputs)
	}
	if len(groundTruths) != 0 {
		t.Fatalf("expected no ground-truth candidates for generic iterator line, got %+v", groundTruths)
	}
}

func TestExtractInputGTCandidatesFromLinePreservesSpecificIdentifiers(t *testing.T) {
	line := "pred = model(images); loss(pred, labels)"
	inputs, groundTruths := extractInputGTCandidatesFromLine(line)
	if !reflect.DeepEqual(inputs, []string{"image"}) {
		t.Fatalf("expected image input candidate, got %+v", inputs)
	}
	if !reflect.DeepEqual(groundTruths, []string{"classes"}) {
		t.Fatalf("expected classes ground-truth candidate, got %+v", groundTruths)
	}
}
