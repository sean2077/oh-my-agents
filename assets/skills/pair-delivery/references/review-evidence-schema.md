# Review evidence schema

Use this operational subset when constructing the machine-checked block for a
`kind: review`. The repository protocol and schema specs remain the development
authority; this installed reference is complete for publishing review evidence.

## Required block

The review body carries exactly one fenced `oma-review-evidence/1` JSON object.
Unknown fields, trailing JSON, missing or repeated fences, empty values, and obvious
placeholders are rejected. Copy the structure below, but replace every example value
with evidence from the current review; never publish the example verbatim.

```oma-review-evidence/1
{
  "schema": "oma-review-evidence/1",
  "findings": [],
  "basis_refs": [
    {"type": "repo", "ref": "README.md:1"}
  ],
  "commands_run": ["go test ./... -> pass"],
  "limitations": ["cross-host end-to-end behavior was not exercised"]
}
```

All five top-level fields are part of the payload. `findings` entries have
`severity`, `confidence`, `claim`, and one or more `refs`. A ref has `type`, `ref`,
and optional `version_or_date`.

## Verdict requirements

- `approve`: `basis_refs`, `commands_run`, and `limitations` must each be non-empty;
  `findings` may be empty. Record commands actually run or a concrete reason they
  were not run, and state what was not checked.
- `revise` or `approve-with-changes`: `findings` must contain at least one finding.
  Keep the other evidence arrays in the payload and populate them from the review.

## Closed enums and references

- severity: `critical|high|medium|low`
- confidence: `high|medium|low`
- ref type: `repo|official|source_reference|supplemental`
- a `repo` ref is a repository-relative `path:line[-line]`; absolute paths and `..`
  are refused
- every other ref type uses an `http://` or `https://` URL; add `version_or_date`
  when currency or version affects the claim
- every finding needs at least one ref; claims, refs, commands, and limitations must
  be concrete rather than placeholders

Pass the matching typed verdict to `oma relay publish`. The CLI canonicalizes the
JSON, computes `evidence_hash`, and stamps it into review frontmatter; do not add or
edit that hash by hand.
