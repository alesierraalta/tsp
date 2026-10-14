# Locale key lookup

`createLookup(entries)` returns a lookup whose keys are matched without regard to case.
Matching is case-insensitive including ß/SS and dotted İ: "straße" matches "STRASSE", and dotted İ is
the plain letter i, so "İstanbul", "ISTANBUL" and "istanbul" are one key.
Diacritics are significant: "café" does not match "cafe".
