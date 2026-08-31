package app

import (
	"encoding/json"
	"testing"
)

func TestGUIResponsesUseLowerCamelCaseJSON(t *testing.T) {
	packData, err := json.Marshal(PackResponse{ProjectPath: "project.json", OutputDir: "out", Warnings: []string{"warning"}})
	if err != nil {
		t.Fatal(err)
	}
	var pack map[string]any
	if err := json.Unmarshal(packData, &pack); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"projectPath", "outputDir", "warnings"} {
		if _, ok := pack[key]; !ok {
			t.Fatalf("PackResponse is missing JSON field %q: %s", key, packData)
		}
	}
	for _, key := range []string{"ProjectPath", "OutputDir", "Warnings"} {
		if _, ok := pack[key]; ok {
			t.Fatalf("PackResponse unexpectedly contains Go field %q: %s", key, packData)
		}
	}

	unpackData, err := json.Marshal(UnpackResponse{})
	if err != nil {
		t.Fatal(err)
	}
	var unpack map[string]any
	if err := json.Unmarshal(unpackData, &unpack); err != nil {
		t.Fatal(err)
	}
	if _, ok := unpack["report"]; !ok {
		t.Fatalf("UnpackResponse is missing JSON field report: %s", unpackData)
	}
}
