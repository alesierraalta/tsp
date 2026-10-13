package worker

// Process returns one result for each distinct input.
func Process(inputs []string) []string {
	seen := make(map[string]struct{})
	unique := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if _, exists := seen[input]; exists {
			continue
		}
		seen[input] = struct{}{}
		unique = append(unique, input)
	}

	byInput := make(map[string]string)
	for _, input := range unique {
		byInput[input] = input
	}

	output := make([]string, 0, len(unique))
	for _, input := range unique {
		output = append(output, byInput[input])
	}
	return output
}
