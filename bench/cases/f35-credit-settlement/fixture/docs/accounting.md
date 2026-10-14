# Accounting conventions

All inputs and results must fit JavaScript safe integers. There is no narrower accounting limit.
Reports preserve the ledger balance exactly, including imported large account balances.
A quote is an unrounded microcredit amount, not a payable line total. Product totals count units.
Currency conversion rounds to nearest integer with Math.round; allocation gives residual credits
in input order. Ledger accessors return copies. Catalog definitions replace previous rates.
