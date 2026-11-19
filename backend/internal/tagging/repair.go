package tagging

const RepairPolicyVersion = "validation-repair-v1"

const RepairInstruction = "\n\nVALIDATION CORRECTION\nA previous response failed validation. Re-read the original posting and return a complete replacement JSON object. All original taxonomy and evidence rules still apply. Use only the supplied source fields. Do not quote the examples or invent evidence. "

func repairInstruction(reason string) string {
	var feedback string
	switch reason {
	case "tag_json_invalid", "validation_taxonomy", "validation_cities", "validation_city_status":
		feedback = "Return every required field with the exact schema types and enum values. cities must be an array; location_status is city if and only if that array is nonempty."
	case "validation_role_evidence_absent", "validation_role_evidence_missing":
		feedback = "role_evidence must be one exact, contiguous quote from the supplied title or description. Copy its spelling, punctuation and spacing exactly. Do not combine separate phrases, add ellipses, rewrite words or copy an example. A short exact title is acceptable if it supports the selected role."
	case "validation_internship_type":
		feedback = "The source employment_type explicitly contains an internship marker. Under the existing precedence rule, employment_type must be internship, including when a generic title or hours suggest another type. Determine the other fields independently from their evidence."
	case "validation_location_evidence_absent", "validation_location_evidence_missing", "validation_city_evidence_missing":
		feedback = "Copy location_evidence exactly from the supplied work-location or remote statement. Every selected city must appear in that evidence, allowing only the documented city aliases. Separate locations entries may be joined with a semicolon and space. Do not infer an unsupported city."
	case "validation_region_as_city", "validation_city_empty_or_duplicate":
		feedback = "Use only distinct supported municipalities as cities. States, districts and countries belong to region status with an empty cities array unless an applicable remote rule establishes remote status."
	case "validation_hardware_role_evidence":
		feedback = "A hardware posting needs explicit software-development or software-testing duties to support a software role. Quote the actual duty exactly; physical inspection, assembly and soldering belong to Other."
	default:
		return ""
	}
	return Prompt + RepairInstruction + feedback
}
