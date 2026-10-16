# Upload session

An in-memory controller for sequential multipart upload events.

`start()` moves a ready session into uploading. `recordPart(id)` records an acknowledged part only while uploading; duplicate ids count once. `pause()` suspends uploading and `resume()` resumes only a paused session, retaining its acknowledged parts. `fail()` ends the current attempt. `retry()` starts a new attempt only after failure, with no acknowledged parts carried over. `complete(requiredParts)` succeeds only while uploading with at least the requested number of distinct acknowledgements. Callers provide a positive integer part count.

Completed sessions are terminal: subsequent operations return false and leave their state unchanged. Invalid transitions return false without changing state. Successful operations return true. The public `state` and `parts` fields are read-only observations for callers. Events are delivered sequentially.

Run: `node --test`.
