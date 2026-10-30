# <Topic: what was measured, in one line>

Status: reading | baseline
Date: <YYYY-MM-DD>
Change: <commit sha, buildinfo version or skill version, and what it changed>

Records under `docs/measurements/` are evidence, not notes: they are read later by someone deciding
whether a claim still holds, so they are never rewritten in place. A superseded reading gets a new
file and a line here naming its successor.

## Question

<The one bounded question this reading answers. Not "is it better" — better at what, measured how,
against which threshold fixed before the change.>

## Instrument

| field | value |
| --- | --- |
| command | `<exact command, copy-pasted>` |
| commit / version | `<sha, buildinfo version, or skill version>` |
| runs per configuration | `<n>` |
| environment | `<os/arch, machine>` |
| bench only: corpus digest, model, runner, agent-config | `<…>` |

## Result

<The numbers side by side, in a named unit, with the unit repeated in the column header.>

## Verdict

<Accept or reject, against the pre-declared threshold, and the noise band the delta had to clear.
State the reject plainly: a reverted change is a result.>

## What this does not prove

<The honest limit: cases not covered, metrics not measured, the confound, the run that abstained. A
reading without this section reads as a stronger claim than it is.>
