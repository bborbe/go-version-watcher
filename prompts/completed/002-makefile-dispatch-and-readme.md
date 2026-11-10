---
status: approved
spec: [001-cluster-build-arm]
created: "2026-11-10T08:01:07Z"
queued: "2026-11-10T08:01:07Z"
---

<summary>
- `BUILD_TARGET` defaults to `local`, so a bare `make build` behaves exactly as it does today.
- Any other value builds the image inside that cluster, in a Job the existing runner renders and applies.
- The cluster arm refuses a `VERSION` that did not come from the caller, before any Job is applied.
- Both arms compose the same Docker Hub reference, differing only in tag.
- `upload`, `clean` and `buca` are refused for a cluster target; `apply` still runs on both arms.
- The local version guard still fires, and the local tag is unchanged.
- Every `make` target also works where `.git` is masked, so the container can run its own verification.
- The README documents both arms, the Docker Hub constraint and the scratch-tag hazard.
</summary>

<objective>
Add the `local` / cluster split to `Makefile.docker` so that `make build` keeps building locally and `make build BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>` builds the image inside a Kubernetes cluster through the runner already present in `hack/`, make both Makefiles resolvable where `.git` is masked, and document the two arms in `README.md`.
</objective>

<context>
Read the approved spec at `specs/in-progress/001-cluster-build-arm.md` — Desired Behavior items 1-7 (item 6's `ROOTDIR` half is this prompt's), the Constraints section (the `$(origin VERSION)` ordering, the target-scoped `$(error …)` rule, and the `ROOTDIR` fallback in both Makefiles), and the Failure Modes rows for an unknown target, a missing `VERSION`, a short `REF` and a missing secret.

Read these files before changing anything:

- `Makefile.docker` — the current `build`, `check-version-tag`, `check-go-mod`, `upload`, `clean`, `apply` and `buca` targets.
- `Makefile.variables` and `Makefile.precommit` — each opens with `export ROOTDIR ?= $(shell git rev-parse --show-toplevel)`.
- `hack/build-cluster.sh` — the environment it requires (`BUILD_TARGET`, `REPO_URL`, `REF`, `TAG`, `IMAGE`, `PUSH_SECRET`, `DOCKERFILE`, `PUSH`) and the fact that it defaults `NAMESPACE` to `build`.
- `hack/build-job.yaml` — note that it hardcodes `namespace: build` and carries **no** `__NAMESPACE__` token.
- `README.md` — it has a `## Development` section and no build section at all.

Requirements 1-9 spell out the dispatch in full; no external repository is reachable from this container and none is needed. (The upstream shape this was adapted from is `github-update-go-agent`'s `Makefile.docker`, which is not mounted here.)
</context>

<requirements>
1. In `Makefile.docker`, declare the target selector near the top, above the `VERSION` default:

```
# BUILD_TARGET selects where `make build` runs. `local` (the default) uses the
# local Docker daemon; any other value names a cluster alias (sun / nukedev /
# nukeprod) or a kubectl context and builds inside that cluster as a Job
# rendered from hack/build-job.yaml by hack/build-cluster.sh.
BUILD_TARGET ?= local
```

2. Wrap the **existing** local arm in `ifeq ($(BUILD_TARGET),local)` … `else` … `endif`. The local branch keeps, unchanged:
   - `build: check-version-tag check-go-mod`
   - the whole `docker build` recipe including `--platform=linux/amd64`, the build args, and `-t $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)`
   - the `check-version-tag` and `check-go-mod` targets verbatim, including the `ALLOW_UNTAGGED_BUILD` escape hatch

   Do not reword, reorder or "improve" the local recipe. It is the regression gate.

3. The cluster branch defines its own `build` target with **no** `check-version-tag` / `check-go-mod` prerequisites, and does three things in order:
   a. refuses a tag the caller did not supply:
   ```
   	@$(if $(filter command line environment,$(origin VERSION)),,$(error the cluster arm needs an explicit VERSION; pass VERSION=<scratch-tag>))
   ```
   b. echoes the composed image reference on one line before invoking the runner;
   c. invokes the runner with the environment it requires:
   ```
   	BUILD_TARGET='$(BUILD_TARGET)' \
   	REPO_URL='https://github.com/bborbe/go-version-watcher.git' \
   	REF='$(REF)' \
   	TAG='$(VERSION)' \
   	DOCKERFILE='Dockerfile' \
   	IMAGE='$(DOCKER_REGISTRY)/bborbe/$(SERVICE)' \
   	PUSH='true' \
   	PUSH_SECRET='buildkit-push' \
   	hack/build-cluster.sh
   ```
   `REPO_URL` and `PUSH_SECRET` are literals, never knobs. Do **not** pass `NAMESPACE`: `hack/build-job.yaml` hardcodes `namespace: build` and has no `__NAMESPACE__` token, so passing one would only mislead the next reader.

4. Add a **second** `ifeq ($(BUILD_TARGET),local)` block wrapping the existing `upload`, `clean` and `buca` targets where they currently sit — between `check-go-mod` and `apply`. Do not move `apply`; it stays below the block and outside every conditional. In the local branch the three targets keep their current bodies. In the `else` branch each is a **target-scoped** `$(error …)` naming the target as local-only, e.g.:
   ```
   .PHONY: upload clean buca
   upload clean buca:
   	@$(error $@ is local-only; BUILD_TARGET=$(BUILD_TARGET) builds and pushes inside the cluster)
   ```
   A target-scoped `$(error …)` is expanded when the recipe is expanded, so it fires under `make -n`. Do **not** write a blanket parse-time `$(error …)` over the whole non-local block: it would also break `build` for the same `BUILD_TARGET`.

5. Leave `apply` **outside** both conditionals and unchanged — it is the publish-only no-op (`@echo "skip apply — publish-only"`) and must keep running on both arms.

6. Put the `$(origin VERSION)` reasoning in a comment beside the guard, so the next person editing the file does not have to re-derive it: `VERSION` is assigned with `:=` inside `Makefile.docker` itself, so `origin` reads `file` for the default and `command line` / `environment` only when the caller supplied it.

7. In `README.md`, add a `## Build` section that shows both arms with a concrete invocation — `make build` and `make build BUILD_TARGET=<cluster> REF=<full-sha> VERSION=<tag>` — states that `buca` remains local, records that the cluster push credential is Docker Hub only so the arm pushes `docker.io/bborbe/go-version-watcher`, names nuke's `MIRROR_IMAGES` step as what copies that image into the private registry, and warns that `VERSION` must be a scratch tag: a tag that already names a released image is overwritten irreversibly.

8. In `Makefile.variables` and `Makefile.precommit`, make `ROOTDIR` resolve where `.git` is masked. Change the first line of each file from
```
export ROOTDIR ?= $(shell git rev-parse --show-toplevel)
```
to
```
export ROOTDIR ?= $(shell git rev-parse --show-toplevel 2>/dev/null || pwd)
```
Without it, `include $(ROOTDIR)/tools.env` in `Makefile.precommit` resolves to `/tools.env` and every `make` target in this repo — including every probe in `<verification>` — dies at parse time. This is spec AC1's `grep -c '2>/dev/null || pwd' Makefile.variables Makefile.precommit` = 1 for each.

9. Add a bullet under `## Unreleased` in `CHANGELOG.md` for the `BUILD_TARGET` dispatch and the README build section. The existing bullet covers only the runner files and the Dockerfile.

After implementing, re-run `<verification>` and confirm every command's stated result holds; then walk each Acceptance Criterion in the spec and confirm which this prompt satisfies.
</requirements>

<constraints>
- The local arm's command, its tag and the `check-version-tag` behaviour must not change.
- No new knobs beyond `BUILD_TARGET`, `REF` and `VERSION`. Do not copy `BUILD_TIMEOUT`, `BUILD_REPO_URL`, `BUILD_DOCKERFILE`, `BUILD_PUSH_SECRET`, `BUILD_NAMESPACE` or `ALLOW_DEFAULT_VERSION` from the upstream template.
- `SERVICE` is assigned in the root `Makefile` **after** `include Makefile.docker`; that is fine because make expands `$(SERVICE)` in recipes at run time. Do not move it.
- Do not edit `hack/`, `Dockerfile`, `specs/` or `prompts/` — the runner files and module-mode Dockerfile are already in place.
- Do NOT commit — dark-factory handles git.
- `make precommit` must still pass.
- Do not run `kubectl` or any cluster command. The `build` namespace and the `buildkit-push` secret already exist; the real cluster build is the spec's Rung-2, verified by the operator after merge.
</constraints>

<verification>
Run inside the container.

```
make precommit

# The selector exists — expect >= 1
grep -c '^BUILD_TARGET' Makefile.docker

# The cluster branch invokes the runner on a non-comment line — expect >= 1
grep -cE '^[[:space:]]*@?[^#]*hack/build-cluster\.sh' Makefile.docker

# ROOTDIR resolves without git — expect 1 for each
grep -c '2>/dev/null || pwd' Makefile.variables Makefile.precommit

# Local arm unchanged: prints the docker build recipe, no runner, tag expanded — expect exit 0
make -n build VERSION=probe
#   expect: docker.io/bborbe/go-version-watcher:probe, and no hack/build-cluster.sh

# Local guard still fires — expect non-zero and "refusing to build"
make check-version-tag VERSION=v9.9.9
#   note: with .git masked this refuses because git is unavailable, so the tag
#   comparison itself is only exercised on the operator rung.

# The escape hatch still short-circuits the guard — expect exit 0
make check-version-tag VERSION=v9.9.9 ALLOW_UNTAGGED_BUILD=1

# Cluster arm composes the reference and invokes the runner — expect exit 0
make -n build BUILD_TARGET=sun REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe
#   expect: docker.io/bborbe/go-version-watcher:probe, then hack/build-cluster.sh

# Cluster arm refuses a caller-less VERSION — expect non-zero naming VERSION
make -n build BUILD_TARGET=sun REF=0123456789abcdef0123456789abcdef01234567

# Local-only targets refused for a cluster target — expect non-zero each
make -n upload BUILD_TARGET=sun VERSION=probe
make -n clean  BUILD_TARGET=sun VERSION=probe
make -n buca   BUILD_TARGET=sun VERSION=probe

# apply still runs on the cluster arm — expect exit 0 and "skip apply — publish-only"
make -n apply BUILD_TARGET=sun VERSION=probe

# The runner accepts the dispatch's values (validates before any kubectl call)
BUILD_TARGET=sun REPO_URL=https://github.com/bborbe/go-version-watcher.git \
REF=0123456789abcdef0123456789abcdef01234567 TAG=probe \
IMAGE=docker.io/bborbe/go-version-watcher PUSH_SECRET=buildkit-push \
DOCKERFILE=Dockerfile PUSH=true hack/build-cluster.sh
#   expect: exit non-zero with a kubeconfig error ("needs the kubeconfig … which does not exist")
#   or an "unknown BUILD_TARGET" error — never "<NAME> is required" and never a whitelist rejection

# README documents both arms — expect >= 1 each
grep -cE 'BUILD_TARGET=[<A-Za-z0-9]' README.md
grep -ci 'buca.*local' README.md
grep -c 'MIRROR_IMAGES' README.md

# README records the Docker Hub push constraint — expect >= 1
grep -ci 'docker hub' README.md

# CHANGELOG carries the new bullet under Unreleased — expect >= 2
# (the existing runner bullet plus the new one); section-walk, not grep -A
awk '/^## /{sec=$0} sec=="## Unreleased" && /^- /{print}' CHANGELOG.md | wc -l
```
</verification>
