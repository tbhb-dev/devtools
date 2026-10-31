# Watcher oracle

Copied byte for byte and left unedited, `watch-checks.sh` is the frozen watcher from `packages/agents/source/skills/watch-pr/shared/scripts/watch-checks.sh` at `a0347a715fab580ece45edfcb3d13509620459a8` (Git blob `b2969dddf082d62698f2c4957240051ee31b8d6e`).

`TestWatchOracle` pins that blob and the baseline definitions of `dt_gh` and `dt_gh_login`, then copies those unchanged definitions from the shared library into a temporary adjacent `lib.sh`. Because the watcher calls only these two library functions, the copy avoids unrelated library probes and imports and preserves the baseline access and quota behavior. The test contains no shell logic of its own.

Both processes run against the same scripted GitHub CLI responses and a restricted search path, and the fake CLI refuses unrecognized requests. A no-op executable replaces shell sleeps. Go waits use polling intervals of zero and a five-second appearance bound. The divergence case uses a one-second settling bound and interval to count the extra read before timeout. A temporary repository with staged, unstaged and untracked content and a local bare remote lets each run check that their state is unchanged.

One difference from the shell is intentional (#978 AC3): after registration the shell reports `ALL GREEN` for an empty polling snapshot, whereas Go keeps reading until checks return or reports `TIMEOUT` with status 2. The test asserts both outcomes, along with the extra Go read when checks return. Every other case requires identical stdout, stderr, status and check-read counts.

Once #333 and #544 permit retirement, the oracle and its Go parity test go away together with the frozen fallback. No runtime path calls them, and published behavior is unchanged.
