package main

import "testing"

func TestParsePatchResponse(t *testing.T) {
	got, err := parsePatchResponse("```json\n{\"edits\":[{\"old\":\"func x()\",\"new\":\"func x(ctx context.Context)\"}]}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Edits) != 1 || got.Edits[0].Old != "func x()" {
		t.Fatalf("bad patch: %#v", got)
	}
}

func TestValidatePatchResponseAcceptsSmallPatchInsideLargeOldNew(t *testing.T) {
	old := "func x() {\n\tprintln(1)\n\tprintln(2)\n}\n"
	new := "func x(debug bool) {\n\tprintln(1)\n\tprintln(2)\n}\n"
	patch := PatchResponse{Edits: []PatchEdit{{Old: old, New: new}}}
	_, err := validatePatchResponse(old, "add debug bool", patch)
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatePatchResponseAcceptsBroadExactPatch(t *testing.T) {
	old := "func x() {\n\tprintln(1)\n\tprintln(2)\n}\n"
	new := "func x() {\n\tif true {\n\t\tprintln(3)\n\t\tprintln(4)\n\t\tprintln(5)\n\t}\n}\n"
	patch := PatchResponse{Edits: []PatchEdit{{Old: old, New: new}}}
	_, err := validatePatchResponse(old, "change x", patch)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreparePatchResponseAllowsAmbiguousModelPatch(t *testing.T) {
	patch := PatchResponse{Edits: []PatchEdit{{Op: "replace", Old: "age", New: "dob"}}}
	edits, err := preparePatchResponse(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].Old != "age" {
		t.Fatalf("bad prepared edits: %#v", edits)
	}
}

func TestValidatePatchResponseAcceptsInsertAtSelectionEnd(t *testing.T) {
	patch := PatchResponse{Edits: []PatchEdit{{Op: "insert_at_selection_end", Text: "\n    def reset(self):\n        self.value = 0\n"}}}
	edits, err := validatePatchResponse("class Counter:\n    pass\n", "add reset method", patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].Op != "insert_at_selection_end" {
		t.Fatalf("bad edits: %#v", edits)
	}
}

func TestValidatePatchResponseNormalizesDeleteAndInsertOps(t *testing.T) {
	content := "class Person:\n    def __init__(self, name, age, height):\n        self.name = name\n        self.age = age\n        self.height = height\n"
	patch := PatchResponse{Edits: []PatchEdit{
		{Op: "replace", Old: "    def __init__(self, name, age, height):", New: "    def __init__(self, name, dob, height):"},
		{Op: "delete", Old: "        self.age = age\n"},
		{Op: "insert_before", Old: "        self.height = height", Text: "        self.dob = dob\n"},
	}}
	edits, err := validatePatchResponse(content, "remove age and add dob", patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 3 || edits[1].New != "" || edits[2].New != "        self.dob = dob\n        self.height = height" {
		t.Fatalf("bad edits: %#v", edits)
	}
}

func TestValidatePatchResponseExpandsAmbiguousInlineReplace(t *testing.T) {
	content := "class Person:\n    def __init__(self, name, age, height):\n        self.name = name\n        self.age = age\n        self.height = height\n"
	patch := PatchResponse{Edits: []PatchEdit{{Op: "replace", Old: "age", New: "dob"}}}
	edits, err := validatePatchResponse(content, "remove age and add dob", patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 2 {
		t.Fatalf("got %d edits: %#v", len(edits), edits)
	}
	if edits[0].Old != "    def __init__(self, name, age, height):\n" || edits[1].Old != "        self.age = age\n" {
		t.Fatalf("bad expanded edits: %#v", edits)
	}
}

func TestValidatePatchResponseExpandsDuplicateLineWithContext(t *testing.T) {
	content := "class Person:\n    def __init__(self, name, age, height):\n        self.name = name\n        self.age = age\n        self.height = height\n\n    def set_age(self, age):\n        self.age = age\n"
	patch := PatchResponse{Edits: []PatchEdit{{Op: "replace", Old: "        self.age = age\n", New: "        self.dob = dob\n"}}}
	edits, err := validatePatchResponse(content, "remove age and add dob", patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 2 {
		t.Fatalf("got %d edits: %#v", len(edits), edits)
	}
	if edits[0].Old == edits[1].Old {
		t.Fatalf("expected contextual edits, got %#v", edits)
	}
}
