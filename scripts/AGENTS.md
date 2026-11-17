# Developer scripts

- Keep scripts narrow, deterministic and explicit about prerequisites.
- Shell scripts use POSIX sh, `set -eu`, bounded retries and traps for owned temporary resources.
- Integration checks use unique disposable Compose projects. Cleanup may remove only their own volumes/containers.
- Generated code checks compare the before/after content and fail on drift; do not rely on tracked Git state alone.
- Commands are exposed through the root Makefile, with no undocumented setup steps.
