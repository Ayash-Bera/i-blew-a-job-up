package tagging

import (
	"testing"

	"firstrole/internal/models"
)

func TestHardwareRoleEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, title, duty, evidence, role string
		want                              bool
	}{
		{"physical quality check", "Quality Check Intern - Hardware", "Check physical condition, damage, loose mounts, worn cables, cleanliness and log it.", "Quality Check Intern - Hardware", "Quality Engineering", false},
		{"hardware assembly", "Hardware Intern - Robotics", "Build, assemble, test, and troubleshoot the mechanical and electrical systems that keep hundreds of units capturing data daily.", "Build, assemble, test, and troubleshoot the mechanical and electrical systems", "Software Engineering", false},
		{"camera recording check", "Quality Check Intern - Hardware", "Capture a short test recording and review it: cameras correctly aligned, footage clear and stable, audio present, timestamps correct.", "Capture a short test recording and review it", "Quality Engineering", false},
		{"generic technology background", "Hardware Intern", "Our software platform serves businesses. Assemble cameras and solder connectors.", "Assemble cameras and solder connectors", "Software Engineering", false},
		{"device fault codes", "Hardware QC Intern", "Check fault codes and track physical inventory.", "Check fault codes and track physical inventory", "Quality Engineering", false},
		{"hardware testing other", "Quality Check Intern - Hardware", "Check physical condition and inspect cameras.", "Check physical condition and inspect cameras", "Other", true},
		{"hardware assembly other", "Hardware Intern", "Assemble mechanical systems and solder wiring.", "Assemble mechanical systems and solder wiring", "Other", true},
		{"embedded firmware", "Hardware and Embedded Intern", "Develop firmware in C++ for embedded devices and assemble test rigs.", "Develop firmware in C++ for embedded devices", "Software Engineering", true},
		{"embedded programming", "Hardware Intern", "Write C++ control loops for sensors.", "Write C++ control loops for sensors", "Software Engineering", true},
		{"firmware role title", "Hardware and Firmware Engineer Intern", "Develop firmware for sensors.", "Hardware and Firmware Engineer Intern", "Software Engineering", true},
		{"software testing", "Hardware Test Intern", "Test firmware and validate software updates on camera rigs.", "Test firmware and validate software updates", "Quality Engineering", true},
		{"test scripts", "Hardware Test Intern", "Write test scripts for device integration.", "Write test scripts for device integration", "Quality Engineering", true},
		{"nonhardware title", "Software Intern", "Build software that controls hardware.", "Software Intern", "Software Engineering", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := models.RawPosting{Title: tc.title, Locations: []string{"Bengaluru"}, Description: tc.duty}
			tags := valid()
			tags.Role, tags.RoleEvidence, tags.LocationEvidence = tc.role, tc.evidence, "Bengaluru"
			err := Validate(tags, p)
			if (err == nil) != tc.want {
				t.Fatalf("Validate() = %v, want accepted %v", err, tc.want)
			}
			if !tc.want && validationFailureReason(err) != "validation_hardware_role_evidence" {
				t.Fatalf("unexpected rejection reason: %v", err)
			}
		})
	}
}

func TestHardwarePromptInvalidatesPreviousCache(t *testing.T) {
	const previous = "d405d283367dba5b6bb9484c7084b39a00191e11d3e3a8b21ecaf4a4ab5f9048"
	if CacheKey(source(), "test-model") == previous {
		t.Fatal("previous prompt cache survived hardware classification change")
	}
}
