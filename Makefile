PROJECT := longhorn-cli
MACHINE := longhorn
# Define the target platforms that can be used across the ecosystem.
# Note that what would actually be used for a given project will be
# defined in TARGET_PLATFORMS, and must be a subset of the below:
DEFAULT_PLATFORMS := linux/amd64,linux/arm64

.PHONY: build validate test ci package ascii-check-prepare

# Branch base for the ASCII check: a commit or ref whose merge base with HEAD
# starts the range of changed files and commits. Empty means the remote branch
# of github.com/longhorn/cli. Example: make validate UPSTREAM_BASE=upstream/v1.10.x
UPSTREAM_BASE ?=
# Override to test another version of the ASCII checker, e.g. from a fork.
ASCII_CHECKER_URL ?=
VALIDATE_BUILD_ARGS := $(if $(ASCII_CHECKER_URL),--build-arg ASCII_CHECKER_URL=$(ASCII_CHECKER_URL))

build:
	docker buildx build --target build-artifacts --output type=local,dest=. -f Dockerfile .

validate: ascii-check-prepare
	docker buildx build --target validate $(VALIDATE_BUILD_ARGS) -f Dockerfile .

test:
	docker buildx build --target base -t longhorn-cli-test -f Dockerfile .
	docker run longhorn-cli-test ./scripts/test

ci: ascii-check-prepare
	docker buildx build --target ci-artifacts $(VALIDATE_BUILD_ARGS) --output type=local,dest=. -f Dockerfile .

package:
	bash scripts/package

# Compute the files and commit messages to check on the host, where git works
# (also in git worktrees), before the containerized validation runs.
ascii-check-prepare:
	@UPSTREAM_BASE="$(UPSTREAM_BASE)" bash scripts/ascii-check-prepare

.PHONY: buildx-machine
buildx-machine:
	@docker buildx create --name=$(MACHINE) --platform=$(DEFAULT_PLATFORMS) 2>/dev/null || true
	docker buildx inspect $(MACHINE)

# variables needed from GHA caller:
# - REPO: image repo, include $registry/$repo_path
# - TAG: image tag
# - TARGET_PLATFORMS: optional, to be passed for buildx's --platform option
# - IID_FILE_FLAG: optional, options to generate image ID file
.PHONY: workflow-image-build-push workflow-image-build-push-secure
workflow-image-build-push: buildx-machine
	MACHINE=$(MACHINE) PUSH='true' IMAGE_NAME=$(PROJECT) bash scripts/package
workflow-image-build-push-secure: buildx-machine
	MACHINE=$(MACHINE) PUSH='true' IMAGE_NAME=$(PROJECT) IS_SECURE=true bash scripts/package

.DEFAULT_GOAL := ci
