# Credit quotes

Quotes for signed invoice adjustments, in integer minor currency units. Negative adjustments are credits and positive adjustments are charges. Rates are nonnegative integer basis points.

`roundMinor(value)` rounds a finite number to the nearest integer minor unit. Exact halfway values are rounded away from zero, so credits and charges have symmetric rounding. Values and intermediate products must remain within JavaScript's safe numeric range.

`quoteAdjustment(netMinor, rateBasisPoints)` returns `{ netMinor, taxMinor, grossMinor }`. Tax is the signed net amount multiplied by the rate and rounded with `roundMinor`; gross is net plus tax. A credit reverses tax as well as net. No cash settlement or unsigned display conversion is performed here.
