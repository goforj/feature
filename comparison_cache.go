package feature

// comparisonCache owns normalized structured attributes for one evaluation only.
type comparisonCache struct {
	values map[string]comparisonValue
}

// comparisonValue retains failures so repeated negative comparisons still reject invalid input.
type comparisonValue struct {
	value any
	err   error
}

// normalize avoids repeated structured copies while preserving scalar validation and normalization.
func (c *comparisonCache) normalize(attribute string, actual any) (any, error) {
	structured := false
	switch actual.(type) {
	case map[string]any, map[string]string, []any, []string:
		structured = c != nil
	}
	if structured {
		if cached, ok := c.values[attribute]; ok {
			return cached.value, cached.err
		}
	}
	budget := 0
	value, err := cloneValue(actual, 0, &budget, true)
	if structured {
		if c.values == nil {
			c.values = make(map[string]comparisonValue)
		}
		c.values[attribute] = comparisonValue{value: value, err: err}
	}
	return value, err
}
