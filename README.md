# Access reconciler

A command-line tool that keeps application accounts in sync with an HR
snapshot and a version-controlled department → group policy. It prints a
read-only plan by default and writes to the application file only with
`-apply`.

## Requirements

- Go **1.25** or newer (`go.mod`).
- `make` (optional; every target is a thin wrapper around the commands shown).

## Commands

All commands run from the repository root.

```sh
# Sets up the demo/ directory and builds the binary
make setup

# Does a plan run
make plan

# Plans and applies
make apply

# Failure scenario
make failure-demo

# Tests
make test

# Cleans up the demo/ and bin/ directories
make clean
```

`plan` and `apply` use`demo/` for the app and HR data. The version-controlled policy in the project root is used. Run `make setup` to reset the demo data.

Exit status: `0` success, `1` rejected input or failed write, `2` usage error.

The plan goes to stdout. The run log (JSON lines) goes to stderr.

### The failure-demo command

1. Apply with `-fail-on=E004`. The revocations planned before E004's
   `create` succeed and are saved. The `create` fails, everything after it is
   logged as `not_attempted`, and the exit status is `1`.
2. Apply again with no flag. The plan is recomputed from the saved file and
   contains only the remaining work: `create E004`, the grants and `enable E004`.
   Nothing is repeated, and no duplicate account is created.
3. Apply once more. `No changes.`

### Assumptions

- The policy is the current snapshot of managed groups. Removing a group from
  the policy does not revoke it from existing members, as it becomes unmanaged. The app state follows the exercise, and does not record which groups were managed when applied.
- Groups are not secrets, so are currently visible in the run log.

## Operation ordering

The plan is sorted by operation phase, then employee ID, then group, so the
same inputs always produce the same plan:

1. `disable`
2. `remove_group`
3. `create` (a disabled account with no groups)
4. `add_group`
5. `enable`

This ensures that wherever a run stops, access degradation is always the first attempt, with disable having the largest impact.

Apply stops on the first error. Runs take the previous state into account, and a repeat will pick up where the last one stopped.

## Run log

Every record generates a `run_id`. Every action record shows the `employee_id`,
`op` and, where relevant, the `group`. HR records are not logged.

- `run_started` / `run_rejected` / `run_finished`
- `planned`: one per planned action, written before any write
- `orphan`: one per account absent from HR
- `outcome`: one per action during apply, `succeeded`, `failed` (with the
  error) or `not_attempted`

An operation is considered `succeeded` only if it was persisted in the app state successfully. The next run re-plans from whatever the state is.

## Production notes (not implemented)

**Authentication and permissions.** 
This service would have its own least privilege service account, to give it an identity. The permission scope to HR would be read-only access of the employee statuses. For the SaaS, broader account lifecycle (read, create, enable, disable), and adding and removing members to the managed groups. It would ask the platform for a token when it needs to talk to another service - an ID token would take the other service as the audience - and use it as a bearer authorization when calling it. These tokens are short lived, and might need to be refreshed, though the platform's SDK might take care of that for us. It's a bit more complex if the services are spread between different platforms, but the core premise of a short-lived token remains. 

**Safe rollout.**

We can start a monitoring phase, where this service only runs in the default plan mode. The plans and the orphans would need to be reviewed until we are satisfied that the runs produce expected results.

Then we can rollout the apply to a specific department / employee(s) and observe the behaviour and reduce the blast radius.

A low-hanging feature we can add to the service is a guard against bulk revocations that go beyond a threshold. We can bring in the SaaS owner to discuss what the threshold should be.
We can start expanding the rollout phase, and be ready to kill the apply when we observe an issue. 

**Detecting stalled offboarding.**

We set up an alert when an inactive account is still active, or when a managed group is still present for an account after N runs. Additionally, alerts on failed or rejected runs, or no-data alerts would help catch run problems, including stalled offboardings.
We'd need to deal with orphans as well, so an alert when such show up would be useful. Also, bringing in HR and the SaaS app owner to set up a policy for orphans would be advisable.

**Recovery without restoring revoked access.** 

Making sure the app state is in line with the SaaS service, fixing any issues that may have resulted in rejected or failed runs, and doing a run should produce a recovery without causing anyone to receive unwanted access. Observing a plan before the apply would also give us extra confidence in the recovery. Backups should not be restored, to avoid restoring revoked access.

**Stakeholder decision needed before rollout.** 

Besides the above-mentioned contacts, we could discuss the unmanaged group ownership topic with application owner, and who owns changes to the policy. With HR we can define what revocation SLA should apply after HR makes an employee inactive.

## Submission notes

- **Time spent:** Roughly 5 hours spread over 2 days.
- **Unfinished / known limitations:**
  - Duplicate keys in `policy.json` are accepted, and the last one wins
    (`encoding/json` behaviour). Detecting them needs a token-level decoder or
    a list-based policy format.
  - The mass-change guard and the alerting described above are not
    implemented.
  - `-fail-on` without `-apply` is ignored.
- **Tools used:** AI for test fixtures, some docs, the Makefile, refactorings, and as a reviewer.
