# Locale key lookup

`createLookup(entries)` returns a lookup whose keys are matched without regard to case.
Matching is case-insensitive including ß/SS and dotted İ.
Diacritics are significant: "café" does not match "cafe".
