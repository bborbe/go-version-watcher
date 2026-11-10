---
status: completed
spec: [001-cluster-build-arm]
summary: Copied the five pinned upstream hack/ files unmodified and switched the Dockerfile build to module mode, with make precommit passing.
execution_id: f1150fb3-90a8-5676-98da-7c80848a714f-exec-001-hack-runner-and-module-mode-dockerfile
dark-factory-version: dev
created: "2026-11-09T08:15:50Z"
queued: "2026-11-09T08:29:55Z"
started: "2026-11-09T08:29:57Z"
completed: "2026-11-09T08:43:19Z"
branch: dark-factory/cluster-build-arm
---

<summary>
- The cluster build runner and its Job template exist in the repo, copied unmodified from two pinned upstream commits.
- The runner can resolve a cluster alias, render the Job, apply it, wait for it, and report its logs.
- A bootstrap script and a credential template exist for the cluster-side prerequisites, but this change does not run them.
- The image Dockerfile now resolves Go dependencies in module mode.
- A fresh clone with no `vendor/` directory can build the image, which is what the cluster Job does.
- The copied scripts are non-empty and parse, not placeholder stubs.
- The runner's input whitelist and newline checks are preserved because the files are copied verbatim.
</summary>

<objective>
Add the five `hack/` files the dispatch invokes — the cluster build runner, its alias resolver, the parameterized Job template, the prerequisite bootstrap script and the credential template — copied unmodified from the pinned upstream commits, and switch the Dockerfile from vendor mode to module mode so a fresh clone builds without a `vendor/` directory.
</objective>

<context>
Read the approved spec at `specs/in-progress/001-cluster-build-arm.md` — especially Desired Behavior items 2 and 6, the Constraints section (the pinned commits and which file comes from which repo), and the Failure Modes rows for `REF`, an existing Job, an interrupted run, module-proxy/OOM/registry failures and a re-required `vendor/`.

Read these files before changing anything:

- `Dockerfile` — the `go build` line currently uses `-mod=vendor`.
- `Makefile.docker` (after prompt 1) — the cluster branch invokes `hack/build-cluster.sh` and passes `BUILD_TARGET`, `REPO_URL`, `REF`, `TAG`, `DOCKERFILE`, `IMAGE`, `PUSH` and `PUSH_SECRET`.

Coding plugin reference (in-container path): `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md`. Note: that guide's canonical template uses `-mod=vendor`, which assumes `check-go-mod` ran first. This change deliberately diverges: the cluster Job clones the repo with no `vendor/`, so the Dockerfile must use module mode. Do not "fix" it back to vendor mode.
</context>

<requirements>
1. Create `hack/` and copy the three runner files from `agent-task-executor` at the pinned commit `91a7e64e160f30be8af0aea64635e1e11bc86731`:
```
mkdir -p hack
rm -rf /tmp/ate /tmp/gs
git clone https://github.com/bborbe/agent-task-executor.git /tmp/ate
git -C /tmp/ate fetch origin 91a7e64e160f30be8af0aea64635e1e11bc86731
git -C /tmp/ate checkout 91a7e64e160f30be8af0aea64635e1e11bc86731
cp /tmp/ate/hack/build-cluster.sh /tmp/ate/hack/cluster-alias.sh /tmp/ate/hack/build-job.yaml hack/
```
Use a plain clone plus an explicit `git fetch origin <sha>`; a `--depth=1` clone does not contain an arbitrary commit.

2. Copy the two prerequisite files from `go-skeleton` at the pinned commit `eb050aa20bb6194d8af7e47baf8880df789bd1ca`:
```
git clone https://github.com/bborbe/go-skeleton.git /tmp/gs
git -C /tmp/gs fetch origin eb050aa20bb6194d8af7e47baf8880df789bd1ca
git -C /tmp/gs checkout eb050aa20bb6194d8af7e47baf8880df789bd1ca
cp /tmp/gs/hack/build-cluster-bootstrap.sh /tmp/gs/hack/dockerconfig.json.tpl hack/
chmod +x hack/build-cluster.sh hack/cluster-alias.sh hack/build-cluster-bootstrap.sh
```

3. Copy the five files unmodified. Do not edit the runner, the alias resolver, the Job template, the bootstrap script or the credential template. The runner's character whitelist and embedded-newline rejection are the security boundary that keeps caller-supplied `REF`, `TAG` and `IMAGE` from being injected into the manifest and embedded shell; editing the files risks bypassing it. The `agent-task-executor` Job template carries `automountServiceAccountToken: false`, which is why the Job template comes from that repo and not `go-skeleton`.

4. In `Dockerfile`, change the build stage's `go build` line from:
```
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=vendor -ldflags "-s" -a -installsuffix cgo -o /main
```
to:
```
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=mod -ldflags "-s" -a -installsuffix cgo -o /main
```
Change nothing else in the Dockerfile. Keep `ARG DOCKER_REGISTRY=docker.io` — the Job template passes `DOCKER_REGISTRY=docker.io` as a build argument so base images resolve from the public registry inside the cluster.

5. If the clone or fetch of either upstream repository fails, do not fabricate placeholder files. Report the failure and stop; the spec's Assumptions allow the five files to be copied by hand on the host and committed on the branch instead. An empty or invented runner would satisfy a bare `ls` but not the `test -s` / `bash -n` checks or the real cluster build.

6. Do not add a `go mod download` layer, a persistent BuildKit cache, multi-arch output, or any other Dockerfile change. This change is the single `-mod=` flag; the spec's Non-goals forbid the rest.

7. Do not run `hack/build-cluster-bootstrap.sh`, `kubectl`, or any cluster command. The `build` namespace and the `buildkit-push` secret already exist (spec Assumptions); this change only adds the files.
</requirements>

<constraints>
- The five files are copied from the pinned commits above, not from a moving branch, and are not edited after copying.
- `cluster-alias.sh` is required: the runner sources it and fails at that line without it.
- The Dockerfile builds in module mode (`-mod=mod`); it must contain zero occurrences of `-mod=vendor`.
- The local arm's `check-go-mod` prerequisite is unchanged by this prompt (prompt 1 already scoped it to the local branch).
- The Job template passes `DOCKER_REGISTRY=docker.io` as a build argument; do not change that to a private mirror.
- The build container's relaxed sandbox profiles and `automountServiceAccountToken: false` are deliberate; do not edit the template.
- No persistent BuildKit cache, no multi-arch images (`linux/amd64` only), no new knobs.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not touch anything under `specs/` or `prompts/`.
- The real cluster build and the `crane digest` / `docker image inspect` checks are the spec's Rung-2, operator-verified after merge; they are not run here.
</constraints>

<verification>
Run inside the container.

```
make precommit

# The five expected files are present
ls hack/build-cluster.sh hack/build-cluster-bootstrap.sh hack/build-job.yaml hack/cluster-alias.sh hack/dockerconfig.json.tpl

# The three scripts are not empty stubs
test -s hack/build-cluster.sh && test -s hack/cluster-alias.sh && test -s hack/build-cluster-bootstrap.sh

# The three scripts parse
bash -n hack/build-cluster.sh && bash -n hack/cluster-alias.sh && bash -n hack/build-cluster-bootstrap.sh

# Dockerfile is module mode — expect 0
grep -c '\-mod=vendor' Dockerfile

# Integration with prompt 1's dispatch: the runner is the invoked command — expect exit 0
make -n build BUILD_TARGET=sun REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe
#   expect: docker.io/bborbe/go-version-watcher:probe, then hack/build-cluster.sh
```
</verification>
