package pricing

// tierFor returns the index and basis points of the band a cumulative quantity falls in. The last
// band runs to the end of the quantity, so the walk always ends.
func (r Rate) tierFor(quantity int64) (int, int64) {
	for i, tier := range r.Tiers {
		if i == len(r.Tiers)-1 || quantity <= tier.UpTo {
			return i, tier.BasisPoints
		}
	}
	return len(r.Tiers) - 1, r.Tiers[len(r.Tiers)-1].BasisPoints
}
