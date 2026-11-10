---
status: completed
approved: "2026-11-09T06:26:24Z"
generating: "2026-11-10T08:01:16Z"
prompted: "2026-11-10T08:01:16Z"
verifying: "2026-11-10T09:13:07Z"
completed: "2026-11-10T09:13:13Z"
branch: dark-factory/cluster-build-arm
---

## Summary

- This repo's image can only be produced on the Mac: `Makefile.docker` has a single `docker build` path.
- Add a second arm beside it: `make build BUILD_TARGET=<cluster>` renders a Kubernetes Job that clones the repo, builds the image in the cluster and pushes it.
- The local arm keeps its command, its tag and its version guard exactly as they are.
- Both arms publish `docker.io/bborbe/go-version-watcher` — the Docker Hub repository the cluster push credential authenticates against and the repository nuke mirrors into the private registry.
- Two supporting edits make the arm reachable: the image must build from a fresh clone, and the version guard must not block a cluster build.

## Problem

`go-version-watcher` runs on nukeprod, and `nuke/go-version-notifier/Makefile` mirrors `go-version-watcher:$(WATCHER_TAG)` from Docker Hub into `docker.prod.nuke.benjamin-borbe.de:443`. The mirror consumes a Docker Hub image, but nothing in this repo can produce one except the operator's Docker daemon: `Makefile.docker` carries one `build` target, and it runs `docker build` locally.

The arm is not a straight paste of a sibling's. Three properties of this repo make a naive copy fail at runtime rather than at review:

- **`build` is chained to a version guard.** The `build` target is `build: check-version-tag check-go-mod`, and `check-version-tag` refuses unless HEAD's exact tag equals `$(VERSION)`. A cluster build builds a named `REF` with a scratch tag, so the guard exits 1 before any Job is rendered — the build looks broken when it never started.
- **The Dockerfile builds with `-mod=vendor`.** `vendor/` is gitignored and absent from `origin/master`. The local arm's `check-go-mod` prerequisite creates it; a Job that clones the repo has no such prerequisite and fails before it compiles anything.
- **`VERSION` has a default.** `Makefile.docker` assigns `$(shell git describe --tags …)` when the caller did not set one. On the cluster arm that default is the newest release tag, and pushing it over an already-released image is irreversible.

One further gap: the dark-factory pipeline writes `/.dark-factory.lock`, `/.dark-factory.log`, `/prompts/log`, `/specs/log` and the in-progress lock files into this tree, and `.gitignore` names none of them — so they surface as untracked files on every run. Worse, the repo carries no `prompts/` directory at all, and the pipeline aborts with `open prompts/in-progress/<name>.md: no such file or directory` the first time it tries to write a generated prompt.

## Goal

`make build` on this repo produces the image locally exactly as it does today, and `make build BUILD_TARGET=<cluster> REF=<full sha> VERSION=<tag>` produces it inside that cluster as a Kubernetes Job, pushing `docker.io/bborbe/go-version-watcher:<tag>` — the repository the nuke mirror step consumes. Both arms work from the same checkout, and the cluster arm works from a fresh clone with no local preparation.

## Non-goals

- Changing the local arm's command, tag or version guard. All three stay as they are.
- Making a cluster the default build target. `BUILD_TARGET ?= local` keeps the local default; flipping it is a separate goal.
- Renaming the image. `docker.io/bborbe/go-version-watcher` is what nuke mirrors and what the deployment consumes.
- Building at release or tag time. This spec adds the capability; *when* a build is triggered is a separate concern.
- A persistent BuildKit cache. Every cluster build pays the cold-cache cost by design.
- Multi-arch images. `linux/amd64` only, matching the cluster nodes.
- Extracting the shared runner into an upstream template. The files are copied, not shared.
- Knobs beyond `BUILD_TARGET`, `REF` and `VERSION`. The repository URL and the push-secret name are literals in the dispatch: a configurable clone source on a build that mounts a push credential is a hazard with no use case here.

## Acceptance Criteria

- [ ] **The local arm's command, tag and guard are unchanged, and the tree is buildable where `git` is unavailable** — with no `BUILD_TARGET` set, `make -n build VERSION=probe` prints the `docker build` recipe, no reference to `hack/build-cluster.sh`, and the expanded `-t docker.io/bborbe/go-version-watcher:probe` (never the literal `$(VERSION)`); the guard still refuses a mismatched tag — `make check-version-tag VERSION=v9.9.9` exits non-zero and prints `ERROR: refusing to build v9.9.9 from this tree.`; and both Makefiles resolve `ROOTDIR` without git — `grep -c '2>/dev/null || pwd' Makefile.variables Makefile.precommit` returns 1 for each. Evidence: the `make -n` output and its exit code, the guard's exit code and stderr, and both counts. ⚠️ `check-version-tag` is a **recipe-level** guard, so `make -n build` exits 0 and merely echoes the recipe text — the string `refusing to build` appears under `-n` whether or not the guard works. Probe the guard by running the target, never through `-n`. ⚠️ Pass `VERSION=probe` on every `-n` probe: `VERSION` itself defaults from `git describe`, so in the masked-`.git` container an unset `VERSION` expands empty and the tag prints as `docker.io/bborbe/go-version-watcher:` — an assertion that cannot pass where this rung runs. ⚠️ Without the `ROOTDIR` fallback, `include $(ROOTDIR)/tools.env` resolves to `/tools.env` and **every** `make` target in this repo — including every probe in this rung — dies at parse time.
- [ ] **Post-Deploy (Rung-2):** the cluster arm builds and pushes from a fresh clone — `make build BUILD_TARGET=sun REF=<full 40-character sha> VERSION=<scratch-tag>` exits 0 from a tree carrying no `vendor/` directory (`ls vendor` fails), `crane digest docker.io/bborbe/go-version-watcher:<scratch-tag>` resolves to a digest, and `docker image inspect` on that same reference fails on this Mac with `No such image`. Evidence: the exit code, the `ls vendor` failure, the Job name, the build pod's `spec.nodeName`, the resolved digest, and the inspect failure text. ⚠️ Exit 0 plus a local inspect failure is satisfiable by an arm that builds nothing; the Job name and the resolved digest are what distinguish a cluster push from a no-op.
  - `deploy_check:` `kubectlsun -n build get secret buildkit-push -o jsonpath='{.metadata.name}'`
  - `deploy_target:` `buildkit-push`
- [ ] **Both arms compose the same reference** — `make -n build BUILD_TARGET=sun REF=<sha> VERSION=probe` prints the composed `docker.io/bborbe/go-version-watcher:probe` on one line before the runner is invoked. Evidence: the printed line, and the digest above resolving at exactly that repository.
- [ ] **Post-Deploy (Rung-2):** the cluster arm refuses a tag it did not receive from the caller — `make build BUILD_TARGET=sun REF=<sha>` with no `VERSION` on the command line or in the environment exits non-zero and prints a message naming `VERSION`, before any Job is applied. Evidence: the exit code, the message, and `kubectlsun -n build get jobs` showing no new Job for that run.
  - `deploy_check:` `kubectlsun -n build get secret buildkit-push -o jsonpath='{.metadata.name}'`
  - `deploy_target:` `buildkit-push`
- [ ] **The runner files and the module-mode Dockerfile are in place** — `ls hack/` lists `build-cluster.sh`, `build-cluster-bootstrap.sh`, `build-job.yaml`, `cluster-alias.sh` and `dockerconfig.json.tpl`, and each of the three scripts is non-empty (`test -s`); `grep -cE '^[[:space:]]*@?[^#]*hack/build-cluster\.sh' Makefile.docker` returns ≥ 1 on an invocation line rather than a comment (`@`-prefixed and `bash`-prefixed forms both count). `grep -c '\-mod=vendor' Dockerfile` returns 0. Evidence: the five names, the three `test -s` exit codes, the count, and the Dockerfile count. ⚠️ `touch` satisfies a bare `ls` and `bash -n` parses an empty file — the `test -s` is what distinguishes a copy from a stub.
- [ ] **`upload` / `clean` / `buca` stay local, refused for a cluster target, while `apply` still runs** — `make -n upload` prints `docker push`; each of `make -n upload`, `make -n clean` and `make -n buca` with `BUILD_TARGET=sun VERSION=probe` exits non-zero and names the target as local-only; and `make -n apply BUILD_TARGET=sun VERSION=probe` still exits 0 printing `skip apply — publish-only`. The refusals are a target-scoped `$(error …)`, expanded before the recipe runs and therefore visible under `-n`. Evidence: the local `upload` output, all three refusal exit codes with their messages, and the `apply` exit code with its output. ⚠️ A blanket `$(error …)` in the non-local branch would kill `apply` too and still satisfy the three refusals — the `apply` clause is what catches it.
- [ ] **The documented build path and the daemon's directories match reality** — `README.md` shows the exact cluster invocation `make build BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>`, states that `buca` remains local, and records that the push credential is Docker Hub only and that nuke's `MIRROR_IMAGES` step is what copies the image into the private registry; `CHANGELOG.md` carries a `## Unreleased` section with a bullet for this change; the pipeline's directories exist and its runtime artifacts are ignored. Evidence: `grep -cE 'BUILD_TARGET=[<A-Za-z0-9]' README.md` ≥ 1, `grep -ci 'buca.*local' README.md` ≥ 1, `grep -ci 'docker hub' README.md` ≥ 1, `grep -c 'MIRROR_IMAGES' README.md` ≥ 1, `grep -c '^## Unreleased$' CHANGELOG.md` = 1, `ls prompts/.keep prompts/in-progress/.keep prompts/completed/.keep specs/.keep specs/in-progress/.keep specs/completed/.keep` all succeed, and `grep -c '^/\.dark-factory\.lock$\|^/\.dark-factory\.log$\|^/prompts/log$\|^/specs/log$\|^/prompts/in-progress/\*\.lock$\|^/specs/in-progress/\*\.lock$' .gitignore` = 6. ⚠️ The repo has **no** `prompts/` directory today; dark-factory writes `prompts/in-progress/001-….md` and fails the whole lifecycle with `open prompts/in-progress/…: no such file or directory` when it is absent. `specs/` is created by `dark-factory spec approve`, `prompts/` is not — the `.keep` files are what make it survive a fresh clone.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` — format, lint, test and security checks clean
- `bash -n hack/build-cluster.sh && bash -n hack/cluster-alias.sh && bash -n hack/build-cluster-bootstrap.sh` — the copied runners parse
- `test -s hack/build-cluster.sh && test -s hack/cluster-alias.sh && test -s hack/build-cluster-bootstrap.sh` — they are not empty stubs
- `grep -c '2>/dev/null || pwd' Makefile.variables Makefile.precommit` returns 1 for each — `ROOTDIR` resolves without git
- `grep -c '^BUILD_TARGET' Makefile.docker` returns ≥ 1
- `grep -cE '^[[:space:]]*@?[^#]*hack/build-cluster\.sh' Makefile.docker` returns ≥ 1
- `ls hack/` lists the five expected files
- `grep -c '\-mod=vendor' Dockerfile` returns 0
- `make -n build VERSION=probe` prints a `docker build` recipe, no `hack/build-cluster.sh`, and `-t docker.io/bborbe/go-version-watcher:probe` (the tag is expanded, never the literal `$(VERSION)`)
- `make check-version-tag VERSION=v9.9.9` exits non-zero and prints `refusing to build`
- `make -n build BUILD_TARGET=sun REF=<sha> VERSION=probe` prints `hack/build-cluster.sh` and the composed `docker.io/bborbe/go-version-watcher:probe`
- `make -n build BUILD_TARGET=sun REF=<sha>` exits non-zero naming `VERSION`
- `make -n upload BUILD_TARGET=sun VERSION=probe`, `make -n clean BUILD_TARGET=sun VERSION=probe` and `make -n buca BUILD_TARGET=sun VERSION=probe` each exit non-zero naming the target as local-only
- `make -n apply BUILD_TARGET=sun VERSION=probe` exits 0 and prints `skip apply — publish-only`
- `grep -cE '^/(\.dark-factory\.(lock|log)|prompts/log|specs/log|prompts/in-progress/\*\.lock|specs/in-progress/\*\.lock)$' .gitignore` returns 6
- `ls prompts/.keep prompts/in-progress/.keep prompts/completed/.keep specs/.keep specs/in-progress/.keep specs/completed/.keep` all succeed
- `grep -rn 'BUILD_TARGET' --exclude-dir=vendor --exclude-dir=specs --exclude-dir=prompts .` — every reference names the new invocation; no stale build documentation is left behind. The spec and prompt directories are excluded because they always contain the string.

### Operator-executable (runs on the host after PR merge, spec verification ladder)

- `make build BUILD_TARGET=sun REF=<full sha> VERSION=<scratch-tag>` — one real cluster build, Job name and digest recorded
- `crane digest docker.io/bborbe/go-version-watcher:<scratch-tag>` — the push landed
- `make build ALLOW_UNTAGGED_BUILD=1` then `docker image inspect docker.io/bborbe/go-version-watcher:<version>` — the local arm still works

## Desired Behavior

1. **A build target selects the arm.** `BUILD_TARGET` defaults to `local`. With `local`, `make build` behaves exactly as it does today: `check-version-tag` and `check-go-mod` run, `docker build` runs on this machine, and the image is tagged `$(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)`.
2. **Any other value routes to a cluster Job.** With `BUILD_TARGET` set to any target `hack/cluster-alias.sh` resolves (a cluster alias such as `sun`, or a raw kubectl context), `make build` prints the composed image reference, then renders `hack/build-job.yaml` and applies it through `hack/build-cluster.sh`, passing the repository URL and the push-secret name as literals. It waits for the Job and reports success or failure with the Job's own log lines.
3. **The cluster arm needs a caller-supplied tag.** The cluster arm refuses to run unless `VERSION` came from the command line or the environment. It rejects `file` and `undefined` origins, so the Makefile's newest-release-tag default can never be pushed. The refusal names the variable and fires before any Job is created.
4. **Both arms publish to one repository.** The cluster arm passes `IMAGE='$(DOCKER_REGISTRY)/bborbe/$(SERVICE)'` — the same repository the local arm tags, differing only in tag — so the two cannot drift.
5. **The local-only goals stay local.** `upload`, `clean` and `buca` assume a Docker daemon on the invoking host. For a cluster target the image is pushed from inside the Job and nothing lands locally, so they are refused by a target-scoped `$(error …)` in the non-local branch — expanded before the recipe runs, so it is visible under `-n`. `apply` stays the existing publish-only no-op on both arms.
6. **The build works from a clone and inside the container.** The Dockerfile resolves Go dependencies in module mode, so a Job that clones the repo with no `vendor/` directory builds the same way a developer's checkout does. The local arm's `check-go-mod` prerequisite is unchanged. The repo's `make` targets also run where `.git` is masked (`workflow: worktree`), so `ROOTDIR` falls back to the working directory.
7. **The documented build path and the daemon's layout match reality.** The README carries a build section showing both arms, the Docker Hub push constraint and the nuke `MIRROR_IMAGES` coupling; the CHANGELOG records the change under `## Unreleased`; the pipeline's directories exist with `.keep` files and its runtime logs and locks are ignored.

## Constraints

- The local arm's command, tag and `check-version-tag` guard must not change. The guard is narrowed by `BUILD_TARGET` only: a bare `make build` on a mismatched tag must still exit 1.
- `check-go-mod` stays on the local branch only. The cluster arm builds a named `REF`, which need not be the local `HEAD`, so preparing the local working tree there would be meaningless.
- The cluster arm's refusal must fire for `$(origin VERSION)` of `file` **or** `undefined`, and accept only `command line` / `environment`. `VERSION` is assigned with `:=` inside `Makefile.docker` itself, so a guard placed after that assignment sees `file` for the default — the opposite of the trap recorded for repos whose `Makefile` assigns the default after the include.
- The local-only refusal for `upload` / `clean` / `buca` is a target-scoped `$(error …)` inside the non-local branch, not a recipe-level `exit 1` and not a blanket parse-time `$(error …)` over the whole non-local block. A recipe-level refusal is invisible to `make -n`, so the AC could not observe it; a blanket parse-time refusal would also break `build` for the same `BUILD_TARGET` and kill `apply`. The same reasoning is why the `check-version-tag` AC probes the target directly rather than through `-n`.
- The five `hack/` files are copied unmodified from a **pinned commit**, not from a moving branch: `hack/build-cluster.sh`, `hack/cluster-alias.sh` and `hack/build-job.yaml` from **`agent-task-executor`** `91a7e64e160f30be8af0aea64635e1e11bc86731` (its Job spec carries `automountServiceAccountToken: false`, which `go-skeleton`'s does not), and `hack/build-cluster-bootstrap.sh` plus `hack/dockerconfig.json.tpl` from **`go-skeleton`** `eb050aa20bb6194d8af7e47baf8880df789bd1ca`. Both repositories are public, so a clone plus `git fetch origin <sha>` from inside the container reaches the pinned commit — a bare `--depth=1` clone does not contain an arbitrary SHA. `cluster-alias.sh` is required — the runner sources it and fails at that line without it.
- Both `Makefile.variables` and `Makefile.precommit` derive `ROOTDIR` from `git rev-parse --show-toplevel`, and `.dark-factory.yaml` sets `workflow: worktree`, which masks `.git` inside the container. Both files must fall back to the working directory — `export ROOTDIR ?= $(shell git rev-parse --show-toplevel 2>/dev/null || pwd)` — so `include $(ROOTDIR)/tools.env` resolves there. Without it every `make` target in this repo, including every probe in the verification rung, dies at parse time.
- Record the `$(origin VERSION)` reasoning as a comment beside the guard in `Makefile.docker`: the next person editing that file needs it, and this spec will not survive to tell them.
- The Job template passes `DOCKER_REGISTRY=docker.io` as a build argument, so the Dockerfile's base images resolve from the public registry inside the cluster. Do not change that to a private mirror.
- `.gitignore` keeps every entry it has today — including `/vendor/`, which `check-go-mod` would otherwise stage on the next `git add .` — and adds only the dark-factory runtime ignores: `/.dark-factory.lock`, `/.dark-factory.log`, `/prompts/log`, `/specs/log`, `/prompts/in-progress/*.lock`, `/specs/in-progress/*.lock`. It contains no `specs/` or `prompts/` entry today, so nothing is being un-ignored.
- The dark-factory directory layout must exist, not merely be un-ignored: `prompts/`, `prompts/in-progress/`, `prompts/completed/`, `specs/in-progress/` and `specs/completed/` each carry a `.keep`. The repo has no `prompts/` directory at all today, and dark-factory fails the whole lifecycle with `open prompts/in-progress/<name>.md: no such file or directory` when it cannot write there — `dark-factory spec approve` creates `specs/in-progress/`, nothing creates `prompts/`.
- `IMAGE`, `SERVICE`, the registry and the tag must each satisfy the runner's character whitelist; no value may carry a newline.
- The README is the in-repo home for the two-arm topology, the Docker Hub constraint and the nuke coupling; the spec dies after implementation, the README does not.
- No new scenario. The behaviour is reachable by checks on the rendered invocation plus one real cluster build.

## Assumptions

These must hold before the cluster AC can pass; none is created by this spec.

- The `build` namespace and the `buildkit-push` dockerconfigjson secret exist on the target cluster, **and that credential covers the registry the arm pushes to**. Measured on the pilot (2026-11-08): the secret authenticates against Docker Hub, not the private mirror, so the arm pushes `docker.io/bborbe/go-version-watcher` and the private-registry copy comes from nuke's `MIRROR_IMAGES` step. The `deploy_check:` above therefore verifies the precondition, not post-change freshness — nothing in this spec is deployed to the cluster, and `hack/build-cluster-bootstrap.sh` is **not** run by this change: the namespace and secret already exist.
- The repo is public, so the Job's anonymous `git fetch` succeeds; the init container carries no clone credential.
- `crane` is installed on the host running the ACs.
- `REF` exists in the local repository **and on `origin`**: the runner derives `SOURCE_DATE_EPOCH` from the local repository, and the Job's init container fetches the ref from the remote. A commit that exists only locally fails at the clone.
- The build pod can reach the Go module proxy.
- The YOLO container can clone the two public upstream repositories at the pinned SHAs to copy `hack/` from. If it cannot, the five files are copied by hand on the host and committed on the branch.
- `kubectlsun` (or the matching cluster wrapper) is on the host running the ACs. Do not use `kubectl --context` / `--kubeconfig`: the operator's shim refuses it.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `BUILD_TARGET` names an unknown cluster or context | The runner fails before applying anything and lists the accepted values | Re-run with a listed target and confirm the printed reference names the intended cluster; no Job exists to clean up |
| `VERSION` omitted on the cluster arm | Refusal before the Job is created, naming `VERSION` | Re-run passing `VERSION=<scratch-tag>`; `kubectlsun -n build get jobs` shows no Job for the failed run |
| `VERSION` resolves to the newest release tag | Refused by the same guard — `origin` is `file` | Pass `VERSION` on the command line and confirm `make -n build … VERSION=probe` prints the composed reference |
| `REF` is a short SHA, or is not in the local repository | The runner dies resolving `SOURCE_DATE_EPOCH` | `git fetch`, then re-run with the full 40-character SHA |
| `REF` exists locally but was never pushed | The clone init container fails: the ref does not exist on `origin` | Push the branch or commit, then re-run |
| The clone's commit epoch differs from the locally derived one | The build container refuses before `buildctl` runs and names both epochs | `git fetch`, then re-run |
| A Job with the derived name already exists in `build` | The runner refuses and names the existing Job rather than overwriting it | Delete the finished Job (`kubectlsun -n build delete job <name>`), then re-run |
| The runner is interrupted after `apply` | The Job keeps running in the cluster while the local invocation exits; the next run hits the existing-Job refusal | Inspect `kubectlsun -n build get jobs`, read the pod's log, delete the Job once finished |
| The build pod cannot reach the module proxy | The Job fails; the runner prints the last 100 log lines and the Job name | Inspect the Job logs; the local arm is unaffected |
| The build pod is OOM-killed or throttled | The Job reports `failed`; the pod status carries `OOMKilled` or a CPU-throttling event | Raise the template's limits in the same change, or build locally for that one image |
| `vendor/` is required by the Dockerfile again | The cluster build fails compiling, with no module download attempted | Restore module mode in the Dockerfile and confirm `grep -c '\-mod=vendor' Dockerfile` returns 0 |
| The registry rejects or throttles the push | The build fails at the push step with the registry's own error | Re-check the secret with `kubectlsun -n build get secret buildkit-push -o jsonpath='{.metadata.name}'`, then re-run |
| The upstream `hack/` files changed since the pinned copy | The runner's assumptions no longer hold; the build fails inside the Job or renders a Job the cluster rejects | Re-copy from the pinned SHAs in Constraints and re-run `bash -n` on the three scripts |
| A caller-supplied `VERSION` names an already-released tag | No guard — the push replaces that tag **irreversibly**; the previous image is recoverable only by rebuilding from that tag's commit | Rebuild and re-push from the released tag's commit, then cut a new release |
| The local arm regresses | `make build` fails or tags differently | Revert the arm and confirm `make -n build` prints the original `docker build` recipe; the local path is the regression gate |

## Security / Abuse Cases

- The runner substitutes caller-supplied values into a manifest and into embedded shell, so every value is checked against a character whitelist and rejected for embedded newlines before rendering. Do not add a value path that bypasses that check.
- The repository URL and the push-secret name are literals in the dispatch, not configurable inputs. The whitelist constrains the *shape* of a URL, not *which* repository is cloned and built with a mounted push credential.
- The build Job mounts a push credential from a `build`-namespace secret and runs with `automountServiceAccountToken: false`. The credential is scoped to pushing an image; it grants nothing else.
- The build container runs as an unprivileged user with sandbox profiles relaxed because rootless BuildKit requires it. That relaxation is deliberate and must not be copied to the clone init container.
- `VERSION` is caller-controlled and becomes an image tag and a Job name. The tag is whitelisted and the Job name is sanitized to a valid DNS name, truncated, and stripped of trailing separators.
- `hack/dockerconfig.json.tpl` names a TeamVault lookup key, never a credential value. Never print the rendered JSON and never commit it.

## Suggested Decomposition

Prompts should be generated in this order — each row is a single prompt with a clear scope.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | `Makefile.docker` dispatch: the `BUILD_TARGET` split, the caller-supplied `VERSION` guard, the composed-reference echo, local-only `upload`/`clean`/`buca`, and the `ROOTDIR` fallback in both Makefiles | 1, 3, 4, 5, 6 | 1, 3, 4, 6 | — |
| 2 | The five `hack/` files and the module-mode Dockerfile (the clone half of DB6) | 2 | 2, 5 | prompt 1 (the dispatch calls the runner) |
| 3 | Repo hygiene and docs: the `.gitignore` runtime ignores, the README build section, the CHANGELOG bullet | 7 | 7 | prompt 1 (its `make precommit` probe needs the `ROOTDIR` fallback) |

Rationale: prompt 1 establishes the dispatch, the guard and the `ROOTDIR` fallback that every later `make` probe depends on; prompt 2 supplies the runner the dispatch calls; prompt 3 touches no build logic but cannot run its own `make precommit` probe until prompt 1 has landed.

## Do-Nothing Option

Leaving this repo without a cluster arm keeps one nukeprod workload's image tied to the Mac. Every weekly rebuild, every release cut while the operator is away from this machine, and every future unattended pipeline needs a human at the keyboard for this one repo. The cost is not the one build; it is that the automation has a hole shaped exactly like the repo nobody is watching.
