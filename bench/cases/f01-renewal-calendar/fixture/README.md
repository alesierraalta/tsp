# Renewal calendar

`monthlyRenewal(day)` and `annualRenewal(day)` accept valid four-digit ISO calendar dates and return the next renewal date as YYYY-MM-DD. Monthly renewals advance one calendar month; annual renewals advance one calendar year. Both preserve the day number when available and otherwise use the last day of the destination month. Calculations use UTC and do not depend on the host timezone. Inputs are not mutated.

Run `node --test`. No dependencies are required.
