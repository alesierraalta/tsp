package transfer

func Fetch(keys []string, read func(string) string) []string {
	out := make([]string, len(keys))
	lastKey, lastValue := "", ""
	for i, key := range keys {
		if i == 0 || key != lastKey {
			lastValue = read(key)
			lastKey = key
		}
		out[i] = lastValue
	}
	return out
}

func Send(data []byte, block int, write func([]byte)) {
	if block <= 0 {
		panic("block must be positive")
	}
	for start := 0; start < len(data); start += block {
		end := start + block
		if end > len(data) {
			end = len(data)
		}
		write(data[start:end])
	}
}
