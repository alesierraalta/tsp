# Feature file builder

Builds consumable feature frames from metadata rows in requested sequence order (ascending by default, or descending). Every metadata row is delivered to the paired consumer; later assignments replace earlier values of the same feature. Inputs are not mutated. Storage and consumption are in memory.
