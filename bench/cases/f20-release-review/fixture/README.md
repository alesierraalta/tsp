# Release review

`Release(text)` tracks a versioned publication. `edit(text)` advances the revision;
`approve()` records review of the current revision. The state label records the
most recent workflow milestone and edits and suspension do not rewrite it.
`publish()` returns true and marks the release published only when the current
revision has been reviewed and publication is not suspended; rejection returns
false without changing the state. `suspend()` blocks publication until `resume()`.
Review and suspension are independent: approval does not resume publication.
