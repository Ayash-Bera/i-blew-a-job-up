package db

import "firstrole/internal/models"

const TagCachePruneBatchSize = 1000

func cacheProtection(keys []string, cohort *models.InitialCohort) map[string]bool {
	keep := make(map[string]bool, len(keys))
	for _, key := range keys {
		keep[key] = true
	}
	if cohort != nil {
		for _, key := range cohort.Versions {
			keep[key] = true
		}
	}
	return keep
}
