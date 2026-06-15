# Bugs

## fff.nvim SIGSEGV from background watcher git-status update

Date observed: 2026-06-15

### Summary

Neovim crashes with `SIGSEGV` shortly after opening/editing a file in a small Git worktree. The crash occurs inside `fff.nvim`'s native Rust shared library, without the daemon/model doing anything. The last `fff.nvim` log message before the crash is consistently a background watcher event followed by a git-status refresh for one changed file.

### Environment

- OS/host: Arch Linux on `optimus`
- Neovim command line: `nvim --embed file_ops.py`
- Plugin: `dmtrKovalenko/fff.nvim`
- Checked-out version:
  - `0f5ead1 (HEAD -> main, tag: 0.6.5-nightly.0f5ead1, origin/main, origin/HEAD) fix: Do not treat single token as a path name prefitler (#421)`
- Native library involved:
  - `/home/evan/.local/share/nvim/lazy/fff.nvim/target/release/libfff_nvim.so`
- Worktree path:
  - `/home/evan/workspaces/daemon/demo`

### Reproduction context

The crash was first noticed while using a Neovim-hosted editing workflow, but it reproduced again without starting the daemon/model. The common factor was simply opening/editing `file_ops.py` while `fff.nvim` had been eagerly loaded at startup (`lazy = false`).

The demo file was a small Python file like:

```python
def read_file(path: str) -> str:
    pass


def write_file(path: str) -> str:
    pass
```

After a file change/write in the worktree, Neovim crashed.

### Evidence from coredumps

Recent coredumps:

```text
TIME                          PID  UID  GID SIG     COREFILE EXE            SIZE
Mon 2026-06-15 19:34:07 IST 46082 1000 1000 SIGSEGV present  /usr/bin/nvim 16.2M
Mon 2026-06-15 19:44:06 IST 48813 1000 1000 SIGSEGV present  /usr/bin/nvim 14.6M
```

`coredumpctl info 48813` showed:

```text
PID: 48813 (nvim)
Signal: 11 (SEGV)
Command Line: nvim --embed file_ops.py

Module /home/evan/.local/share/nvim/lazy/fff.nvim/target/release/libfff_nvim.so without build-id.
Module libfff_nvim.so without build-id.

Stack trace of thread 48845:
#0  0x00007f9612372040 n/a (libc.so.6 + 0x172040)
#1  0x00007f9610fd91cf n/a (libfff_nvim.so + 0x1d91cf)
#2  0x00007f9610fa7ec1 n/a (libfff_nvim.so + 0x1a7ec1)
#3  0x00007f9610fc193a n/a (libfff_nvim.so + 0x1c193a)
#4  0x00007f9611030865 n/a (libfff_nvim.so + 0x230865)
#5  0x00007f9610ffa9bd n/a (libfff_nvim.so + 0x1fa9bd)
#6  0x00007f9611005ff8 n/a (libfff_nvim.so + 0x205ff8)
#7  0x00007f96112fc83f n/a (libfff_nvim.so + 0x4fc83f)
#8  0x00007f96122981b9 n/a (libc.so.6 + 0x981b9)
#9  0x00007f961231d21c n/a (libc.so.6 + 0x11d21c)

Stack trace of thread 48813:
#0  0x00007f96122a0a52 n/a (libc.so.6 + 0xa0a52)
#1  0x00007f9612294abc n/a (libc.so.6 + 0x94abc)
#2  0x00007f9612294b04 n/a (libc.so.6 + 0x94b04)
#3  0x00007f961231d495 epoll_pwait (libc.so.6 + 0x11d495)
#4  0x00007f96124f68ad n/a (libuv.so.1 + 0x2a8ad)
#5  0x00007f96124e1b92 uv_run (libuv.so.1 + 0x15b92)
#6  0x0000561340dd865f loop_poll_events (/usr/bin/nvim + 0x16365f)
#7  0x0000561340f2f3e7 n/a (/usr/bin/nvim + 0x2ba3e7)
#8  0x0000561340f2fb11 input_get (/usr/bin/nvim + 0x2bab11)
#9  0x0000561340fd0498 state_enter (/usr/bin/nvim + 0x35b498)
#10 0x0000561340ef5b96 normal_enter (/usr/bin/nvim + 0x280b96)
#11 0x0000561340ccd448 main (/usr/bin/nvim + 0x58448)
#14 0x0000561340ccf0b5 _start (/usr/bin/nvim + 0x5a0b5)
```

The earlier coredump (`46082`) had the same pattern: crashed thread inside `libfff_nvim.so`, main Neovim thread waiting in the event loop.

### Last fff.nvim logs before crash

From `~/.local/state/nvim/fff.log` immediately before the crash:

```text
2026-06-15T18:43:44.672447Z  INFO fff_search::background_watcher: Initializing background watcher for path: /home/evan/workspaces/daemon/demo, mode: Neovim
2026-06-15T18:43:44.672540Z  INFO fff_search::background_watcher: File watcher initialized for 0 directories (NonRecursive) under /home/evan/workspaces/daemon/demo
2026-06-15T18:43:44.672586Z  INFO fff_search::background_watcher: Background file watcher initialized successfully
2026-06-15T18:43:44.672602Z  INFO fff_search::file_picker: Background file watcher initialized successfully
2026-06-15T18:43:44.672604Z  INFO fff_search::file_picker: Cache budget configured for 0 files: max_files=30000, max_bytes=536870912
2026-06-15T18:43:44.672618Z  INFO fff_search::file_picker: Warmup completed in 0.00s (cached 0 files, 0 bytes)
2026-06-15T18:43:44.672624Z  INFO fff_search::file_picker: Building bigram index for 0 files...
2026-06-15T18:43:44.672937Z  INFO fff_search::file_picker: Bigram index built in 0.00s — 0 dense columns for 0 files
2026-06-15T18:43:44.672946Z  INFO fff_search::file_picker: Post-scan phase total: 0.00s (warmup=true, content_indexing=true)
2026-06-15T18:44:04.683896Z  INFO fff_search::background_watcher: Event processing summary: 0 to remove, 1 to add/modify, 0 new dirs
2026-06-15T18:44:04.683931Z  INFO fff_search::background_watcher: apply_changes complete: 1 files to update git status
2026-06-15T18:44:04.683932Z  INFO fff_search::background_watcher: Fetching git status for 1 files

=== CRASH SIGSEGV ===
signal 11
   0: <unknown>
   1: <unknown>
   2: <unknown>
   3: <unknown>
   4: <unknown>
   5: <unknown>
   6: <unknown>
   7: <unknown>
   8: <unknown>
   9: <unknown>
  10: <unknown>
  11: <unknown>

=== CRASH END SIGSEGV ===
```

A prior crash had the same final sequence:

```text
fff_search::background_watcher: Event processing summary: 0 to remove, 1 to add/modify, 0 new dirs
fff_search::background_watcher: apply_changes complete: 1 files to update git status
fff_search::background_watcher: Fetching git status for 1 files
```

Then `SIGSEGV` in `libfff_nvim.so`.

### Expected behavior

A file change detected by the background watcher should update the index/git status or fail gracefully. Neovim should not crash.

### Actual behavior

Neovim receives `SIGSEGV`. The crashing thread is in `libfff_nvim.so`. The main Neovim thread is idle in the event loop.

### Suspected area

The crash appears to be in the native backend path used by:

- background watcher event handling
- `apply_changes`
- per-file git status refresh after watcher events

The last log line before crash is `Fetching git status for 1 files`, so the libgit2/git-status path for a modified/added file is suspect.

### Workaround used locally

I disabled eager startup of `fff.nvim` by changing my Lazy spec from:

```lua
lazy = false
```

to:

```lua
lazy = true
```

This prevents the background watcher from being started just by opening Neovim. Avoiding `fff.nvim` keys avoids the crash for now.
