# Policy overlay

`Merge(base Config, patch Patch)` produces a configuration without mutating either input.
A nil patch field inherits the corresponding base value. A non-nil field replaces it
with the pointed-to value. Retries is the number of additional attempts, including zero
for no additional attempts. Enabled controls whether the policy runs; false disables it.
Fields are independent and patches can be applied to a previously merged configuration.
The caller owns pointer storage; Merge retains no pointers.

Run `go test ./...`.
