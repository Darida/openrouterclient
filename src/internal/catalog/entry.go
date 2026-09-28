package catalog

// entry is the part of one /api/v1/models item this package reads.
type entry struct {
	ID                  string   `json:"id"`
	SupportedParameters []string `json:"supported_parameters"`
	Architecture        struct {
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
}
