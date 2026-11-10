---
status: completed
spec: [001-cluster-build-arm]
summary: Added BUILD_TARGET dispatch to Makefile.docker (unchanged local arm, cluster Job arm with caller-supplied VERSION guard, shared image reference, local-only refusals) and made ROOTDIR fall back to pwd in both Makefiles, with a CHANGELOG entry.
execution_id: a91eaa40-88bb-525e-b672-2fc510790723-exec-002-spec-001-build-target-dispatch
dark-factory-version: dev
created: "2026-11-10T07:46:48Z"
queued: "2026-11-10T08:02:45Z"
started: "2026-11-10T08:02:48Z"
completed: "2026-11-10T08:17:30Z"
branch: dark-factory/cluster-build-arm
---

<summary>
- A single build target now selects between two arms: the existing local Docker build and a new in-cluster Kubernetes Job build.
- With no build target set, the local arm behaves exactly as it does today, including its command, its image tag and its version guard.
- The cluster arm refuses to run unless the caller supplies the tag, so the Makefile's newest-release-tag default can never be pushed over an existing release.
- Both arms compose the same image repository reference, so they cannot drift.
- The cluster arm prints the composed reference before it hands off to the cluster runner.
- Local-only goals such as the publish, clean and one-shot publish targets are refused for a cluster target with a clear message, while the publish-only apply goal still works on both arms.
- The repo's make targets now work where git metadata is unavailable, which is how the automated container runs.
</summary>

<objective>
Add the `BUILD_TARGET` dispatch to `Makefile.docker`: keep the local arm byte-for-byte, add the cluster arm that renders and runs the Kubernetes build Job, guard the cluster arm against a caller-less tag, refuse the local-only goals for a cluster target, and make `ROOTDIR` fall back to the working directory in both Makefiles so the tree is usable where `.git` is masked.
</objective>

<context>
Read the approved spec at `specs/in-progress/001-cluster-build-arm.md` — especially Desired Behavior items 1-5, the Constraints section (the `$(origin VERSION)` trap, the target-scoped refusal, the unchanged local arm), and the Failure Modes rows for a missing `VERSION`, an unknown target and a local-arm regression.

Read these files before changing anything:

- `Makefile.docker` — the file being changed. Note the `VERSION` default block, the `build` target and its prerequisites, and the `upload` / `clean` / `apply` / `buca` targets.
- `Makefile.variables` and `Makefile.precommit` — both derive `ROOTDIR` from `git rev-parse --show-toplevel`. `Makefile.precommit` includes `$(ROOTDIR)/tools.env`, so a broken `ROOTDIR` breaks every make target at parse time.
- `Makefile` — defines `SERVICE = go-version-watcher` after including `Makefile.docker`; `$(SERVICE)` is therefore available when recipes expand.
- `hack/build-cluster.sh` — the runner the cluster arm invokes. Read its header and its `require` calls: it needs `BUILD_TARGET`, `REPO_URL`, `REF`, `TAG`, `IMAGE` and `PUSH_SECRET` in the environment, and accepts optional `DOCKERFILE`, `PUSH`, `NAMESPACE`, `TIMEOUT` and `DOCKER_REGISTRY`. It validates every value against a character whitelist and rejects embedded newlines. Do not reimplement that validation in make and do not pass values it does not accept.
- `.dark-factory.yaml` — `workflow: worktree` masks `.git` inside the container. Do not rely on git for verification.

Reference docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-makefile-commands.md`
- `docs/dod.md` (the project's definition of done)

The five `hack/` files and the module-mode `Dockerfile` already exist from completed prompt `001-hack-runner-and-module-mode-dockerfile`. This prompt only changes the Makefiles; do not touch `hack/` or `Dockerfile`.
</context>

<requirements>
1. In `Makefile.variables`, change the single `ROOTDIR` line from:
```
export ROOTDIR ?= $(shell git rev-parse --show-toplevel)
```
to:
```
export ROOTDIR ?= $(shell git rev-parse --show-toplevel 2>/dev/null || pwd)
```

2. In `Makefile.precommit`, make the identical change to its first line:
```
export ROOTDIR ?= $(shell git rev-parse --show-toplevel 2>/dev/null || pwd)
```
It must remain the first line, before `include $(ROOTDIR)/tools.env`. Without the fallback, `ROOTDIR` resolves to `/` in the masked-`.git` container and every make target dies at parse time resolving `/tools.env`.

3. In `Makefile.docker`, add `BUILD_TARGET ?= local` immediately after the existing `DOCKER_REGISTRY ?= docker.io` line. Do not change the `DOCKER_REGISTRY` line.

4. Leave the `VERSION` default block exactly as it is:
```
ifeq ($(VERSION),)
	VERSION := $(shell git describe --tags `git rev-list --tags --max-count=1`)
endif
```
It must stay above the dispatch and must not gain `2>/dev/null` or any other edit: the local arm's tag derivation is part of the unchanged local behavior.

5. Replace the single `build` target with a `BUILD_TARGET` conditional. The local branch keeps the existing target, prerequisites and recipe byte-for-byte. The cluster branch validates the tag origin, prints the composed reference, then invokes the runner with the literal repository URL and push-secret name. The exact target-state for this part of the file is:
```make
# A cluster build must never push the Makefile's newest-release-tag default.
# VERSION is assigned with := in this file, so a guard placed after that
# assignment sees `file` for the default. Accept only values the caller
# supplied: `command line` or `environment`. Pushing over an existing release
# tag is irreversible, so `file` and `undefined` are both refused.
ifeq ($(BUILD_TARGET),local)
.PHONY: build
build: check-version-tag check-go-mod
	@if [ -z "$(VERSION)" ]; then echo "ERROR: no VERSION set and no git tag found — set VERSION=vX.Y.Z or create a tag" >&2; exit 1; fi
	DOCKER_BUILDKIT=1 \
	docker build \
	--pull \
	--no-cache \
	--rm=true \
	--platform=linux/amd64 \
	--build-arg DOCKER_REGISTRY=$(DOCKER_REGISTRY) \
	--build-arg BUILD_GIT_COMMIT=$$(git rev-parse --short HEAD) \
	--build-arg BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
	-t $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION) \
	-f Dockerfile .
else
.PHONY: build
build:
	$(if $(filter command line environment,$(origin VERSION)),,$(error VERSION must be set on the command line or in the environment for a cluster build (BUILD_TARGET=$(BUILD_TARGET))))
	@echo "$(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)"
	BUILD_TARGET=$(BUILD_TARGET) \
	REPO_URL=https://github.com/bborbe/go-version-watcher.git \
	REF=$(REF) \
	TAG=$(VERSION) \
	DOCKERFILE=Dockerfile \
	IMAGE=$(DOCKER_REGISTRY)/bborbe/$(SERVICE) \
	PUSH=true \
	PUSH_SECRET=buildkit-push \
	hack/build-cluster.sh
endif
```
Notes that are load-bearing:
- The guard is a recipe-line `$(if …)` whose else branch is `$(error …)`. Make expands it before running the first recipe line and also under `-n`, so the refusal is observable with `make -n` and fires before any Job is created. It must accept only `$(origin VERSION)` values `command line` and `environment`; `file` (the newest-release-tag default) and `undefined` are refused.
- `TAG=$(VERSION)` and `IMAGE=$(DOCKER_REGISTRY)/bborbe/$(SERVICE)` compose the same repository the local arm tags; the two arms must not use different repositories.
- `REPO_URL` and `PUSH_SECRET=buildkit-push` are literals, not variables and not caller inputs. Do not add a configurable clone source or secret name.
- Pass only the variables listed. The runner defaults `NAMESPACE=build`, `TIMEOUT=3600`, `DOCKERFILE=Dockerfile` and `DOCKER_REGISTRY=docker.io`; do not restate or override them.

6. Keep `check-version-tag` and `check-go-mod` defined exactly as they are today. They stay prerequisites of the local `build` only. Do not narrow, weaken or move the version guard: a bare `make build` on a mismatched tag must still exit 1.

7. Keep `apply` defined once, outside the `BUILD_TARGET` conditional, with its current comment and recipe:
```make
# Publish-only: no k8s in this repo. Deployment lives in the maintainer Helm
# chart (mirror + keel-pattern). apply is a no-op placeholder.
.PHONY: apply
apply:
	@echo "skip apply — publish-only"
```

8. Wrap `upload`, `clean` and `buca` in the same `BUILD_TARGET` conditional. The local branch keeps their current recipes and comments. The non-local branch defines each target with a target-scoped `$(error …)` naming the target as local-only, so the refusal is visible under `-n` and does not break `build` or `apply`:
```make
ifeq ($(BUILD_TARGET),local)
.PHONY: upload
upload:
	docker push $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)

.PHONY: clean
clean:
	docker rmi $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION) || true
	docker image prune -f || true
	docker builder prune --all -f || true
	rm -rf vendor

# Operator-run publish to Docker Hub (requires `docker login`). CI runs
# `make precommit`, never `buca`, so the unconditional push is safe.
.PHONY: buca
buca: build upload clean apply
else
.PHONY: upload
upload:
	$(error upload is local-only; BUILD_TARGET=$(BUILD_TARGET) builds and pushes inside the cluster Job)

.PHONY: clean
clean:
	$(error clean is local-only; BUILD_TARGET=$(BUILD_TARGET) builds and pushes inside the cluster Job)

.PHONY: buca
buca:
	$(error buca is local-only; BUILD_TARGET=$(BUILD_TARGET) builds and pushes inside the cluster Job)
endif
```
The refusal must be a target-scoped `$(error …)` in the non-local branch — not a recipe-level `exit 1` (invisible to `-n`) and not a parse-time `$(error …)` over the whole non-local block (which would also kill `build` and `apply`).

9. Do not add any knob beyond `BUILD_TARGET`, `REF` and `VERSION`. Do not default `REF`, do not default `VERSION` on the cluster arm, do not add a clone-source or secret-name variable, and do not add a `BUILD_TARGET` value other than the existing runner-resolved aliases.
</requirements>

<constraints>
- The local arm's command, tag and `check-version-tag` guard must not change. The guard is narrowed by `BUILD_TARGET` only: a bare `make build` on a mismatched tag must still exit 1.
- `check-go-mod` stays on the local branch only; it is not a prerequisite of the cluster arm.
- The cluster arm's refusal must fire for `$(origin VERSION)` of `file` or `undefined` and accept only `command line` / `environment`.
- The local-only refusal for `upload` / `clean` / `buca` is a target-scoped `$(error …)` inside the non-local branch, not a recipe-level `exit 1` and not a blanket parse-time `$(error …)`.
- `apply` must keep working on both arms.
- `ROOTDIR` must fall back to the working directory in both `Makefile.variables` and `Makefile.precommit`.
- The repository URL and push-secret name are literals in the dispatch; they are not configurable inputs.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not touch `hack/`, `Dockerfile`, `.gitignore`, `README.md`, `CHANGELOG.md`, `specs/` or `prompts/`.
</constraints>

<verification>
Run inside the container. `.git` is masked here, so every probe below must work without git metadata.

```
make precommit

# ROOTDIR fallback present once in each file — expect 1 for each
grep -c '2>/dev/null || pwd' Makefile.variables Makefile.precommit

# BUILD_TARGET is declared — expect >= 1
grep -c '^BUILD_TARGET' Makefile.docker

# The dispatch invokes the runner on a non-comment line — expect >= 1
grep -cE '^[[:space:]]*@?[^#]*hack/build-cluster\.sh' Makefile.docker

# Local arm unchanged: prints the docker build recipe, no cluster runner,
# and the expanded tag (never the literal $(VERSION))
make -n build VERSION=probe
#   expect: a `docker build` recipe, no `hack/build-cluster.sh`, and
#   `-t docker.io/bborbe/go-version-watcher:probe`

# The version guard still refuses a mismatched tag (run it, do NOT probe via -n)
make check-version-tag VERSION=v9.9.9
#   expect: non-zero exit and `ERROR: refusing to build v9.9.9 from this tree.`

# Cluster arm: prints the composed reference, then the runner — expect exit 0
make -n build BUILD_TARGET=sun REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe
#   expect: `docker.io/bborbe/go-version-watcher:probe` on one line, then
#   `hack/build-cluster.sh`

# Cluster arm without a caller-supplied tag — expect non-zero naming VERSION
make -n build BUILD_TARGET=sun REF=0123456789abcdef0123456789abcdef01234567

# Local publish target still prints docker push — expect exit 0
make -n upload VERSION=probe
#   expect: `docker push`

# Local-only goals refuse a cluster target — each must exit non-zero and name
# the target as local-only
make -n upload BUILD_TARGET=sun VERSION=probe
make -n clean  BUILD_TARGET=sun VERSION=probe
make -n buca   BUILD_TARGET=sun VERSION=probe

# apply is not local-only — expect exit 0 and `skip apply — publish-only`
make -n apply BUILD_TARGET=sun VERSION=probe
```
</verification>
