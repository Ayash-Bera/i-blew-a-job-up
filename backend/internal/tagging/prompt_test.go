package tagging

import (
	"encoding/json"
	"regexp"
	"testing"

	"firstrole/internal/models"
)

func TestPromptExamplesUseValidSourceEvidence(t *testing.T) {
	examples := regexp.MustCompile(`(?m)^Input: (.+)\nOutput: (.+)$`).FindAllStringSubmatch(Prompt, -1)
	if len(examples) == 0 {
		t.Fatal("no extraction examples found")
	}
	for _, example := range examples {
		var posting models.RawPosting
		if err := json.Unmarshal([]byte(example[1]), &posting); err != nil {
			t.Fatal(err)
		}
		t.Run(posting.Title, func(t *testing.T) {
			var tags models.Tags
			if err := Decode(example[2], &tags); err != nil {
				t.Fatal(err)
			}
			if err := Validate(tags, posting); err != nil {
				t.Fatal(err)
			}
		})
	}
}
