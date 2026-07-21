---
name: privacy-safe-deploy
description: Use when a site, documentation bundle, or static artifact may expose unpublished or sensitive data during an authorized deployment.
---

# privacy-safe-deploy

Promote the exact artifact that passed publication and privacy gates. Build once,
inspect those final bytes, deploy them unchanged, then verify the live result.

## Establish authority

1. **Inspect** repository contracts, build and deploy documentation, Git state,
   remotes, and provider configuration. Identify the source of truth, publication
   rule, final output directory, target project, environment, and branch.
2. **Confirm** that the user authorized the external action. A request to build,
   audit, or prepare does not authorize a deployment, push, release, or upload.
3. **Record** every source revision that contributes publishable content,
   including linked or nested repositories.
4. **Stop** when the target, environment, publication rule, or authorization is
   ambiguous. Request only the missing authority before changing external state.

## Build the candidate

1. **Use** the project-owned locked or reproducible build path and ensure stale
   output cannot survive into the candidate.
2. **Treat** generated output as immutable. When a finding originates in source,
   repair the source of truth and rebuild; never patch the generated bundle.
3. **Reconcile** the publication boundary semantically. Compare eligible,
   included, and filtered source counts with the build report, and surface parse
   failures. Frontmatter text search alone is not evidence that the rule held.
4. **Stop** on any unexplained count mismatch, parser failure, stale artifact, or
   build error. Fix the responsible source or build contract, then rebuild from
   the affected gate.

## Gate the final bytes

Scan the complete output recursively, including HTML, indexes, feeds, manifests,
redirects, bundled data, and static assets. Scan the final artifact even when the
source scan was clean because aggregation and transformation can introduce leaks.

Classify at least these surfaces:

- **Credentials:** private-key markers, provider tokens, API keys, JWTs,
  authorization headers, credential-bearing URLs, and password, token, or secret
  assignments.
- **Local identity:** absolute home or workspace paths, usernames, hostnames,
  internal domains, private addresses, device identifiers, and user-supplied
  sensitive strings.
- **Sensitive files:** environment files, key stores, certificates, password
  databases, credential or secret backups, debug dumps, source maps, and raw
  source that the publication contract excludes.
- **Personal data:** government identifiers with checksum validation where
  available, phone numbers, email addresses, physical addresses, and exact
  personal identifiers.

**Block** every unresolved high-confidence hit. Context-review lower-confidence
matches and record an explicit disposition. Allow only narrow, source-owned,
reviewed exceptions; broad path or file-type exclusions invalidate the gate.
Report rule, count, and masked location or digest without echoing secret values.

If a leak belongs to a linked or separate source repository, stop deployment,
confirm edit authority for that repository, preserve unrelated work, repair and
verify the source there, commit or push only when authorized, then rebuild the
consumer artifact.

## Promote the artifact

Require all of the following before deployment:

- the build and publication-boundary reconciliation are green;
- the privacy scan has zero unresolved blockers;
- representative artifact hashes are recorded;
- the exact target, environment, branch, and account are confirmed;
- required source revisions are committed and pushed without remote drift.

Deploy with the project-owned command. Capture the provider deployment ID,
environment, branch, source revision, immutable deployment URL, and canonical URL.
If the artifact changes after any gate, discard the prior evidence and restart at
the build gate.

## Verify production

1. **Probe** both the immutable deployment URL and canonical URL. Include the
   homepage, a content index or feed, every repaired page, and representative
   static assets.
2. **Compare** each response with the local candidate by byte hash. When the
   provider intentionally transforms content, use the documented semantic
   comparison and state why byte equality is unavailable.
3. **Check** status, content type, redirect destination, and the relevant privacy
   markers on live responses.
4. **Confirm** the provider record names the expected project, production
   environment, branch, and source revision.
5. **Verify** local, tracking, and remote revision parity plus worktree state in
   every touched repository. Report unrelated pre-existing dirt without absorbing
   it into the delivery.

Provider success is not completion. Completion requires live evidence that the
authorized target serves the gated artifact.

## Red Flags / STOP

Stop and return to the named gate when:

- a source-only scan is being substituted for a final-artifact scan;
- generated output is about to be edited instead of its source of truth;
- one unresolved leak is being waived so the rest can deploy;
- provider success is being treated as proof of live artifact integrity;
- deployment is being retried before diagnosing a wrong target, revision, or
  content mismatch.

Name the failed gate, preserve masked evidence, propose the lowest-scope recovery,
and do not claim deployment complete until every gate has fresh passing evidence.
