# kore-plugin-file Usage

A file management plugin (L2+): **19 real file methods** within a controlled root — list, read, atomic write, delete (trash by default), mkdir, stat, upload, download, extract, compress, chmod, move, rename, copy, search, trash list/restore/purge, and batch operations. No external unzip/tar/chmod binaries are bundled; archives use the Go standard library.

## Installation and compatibility

Install the released tar.gz with the kore plugin installer; build standalone with `make build` / `make dist`. Protocol `kore.plugin.v1`, `lifecycle=lazy`, `depth_level=production`, `requires_kore: >=0.1.0 <0.2.0`. Plugin capabilities `system:read` + `system:admin`; method capabilities mark 6 read methods `system:read` and the remaining write/management methods `system:admin`, matching `info.json`. Method names always use `file.<verb>`; bare names are kept only for legacy compatibility.

## Methods

| Method | Capability | Input | Key result |
| --- | --- | --- | --- |
| `file.list` | read | `{path}` | name/path/is_dir/size/mode/mod_time/is_symlink/uid/gid; `.trash` hidden at root |
| `file.read` | read | `{path, max_bytes?}` | UTF-8 content/size/truncated/is_binary; binary returns no content |
| `file.stat` | read | `{path}` | size/mode/owner/mod_time/is_dir/is_symlink |
| `file.download` | read | `{path}` | filename/size_bytes/data_base64; oversized files suggest directory archive |
| `file.search` | read | `{path?,pattern,content?,...}` | scanned/matched/skipped/truncated/entries with line/preview |
| `file.trash.list` | read | `{limit?}` | count/total/skipped/truncated/entries in reverse deletion order |
| `file.write` | admin | `{path,content,create_if_missing?}` | temp file + rename atomic replacement |
| `file.delete` | admin | `{path,recursive?,permanent?}` | trash by default; `permanent=true` deletes directly |
| `file.mkdir` | admin | `{path,mode?}` | default 0755 |
| `file.chmod` | admin | `{path,mode,recursive?}` | whole-tree validation before applying |
| `file.move` / `file.rename` / `file.copy` | admin | source/target or path/name | cross-directory move, rename-only, recursive copy; symlinks rebuilt to original targets |
| `file.upload` / `file.uploadChunk` / `file.uploadStatus` | admin/read | `{path,filename,data_base64,offset?}` | atomic replacement, resumable upload (duplicate offset idempotent) |
| `file.extract` / `file.compress` | admin | `{path,...}` | Go standard library; malicious zip rejected |
| `file.trash.restore` / `file.trash.purge` / `file.batch` | admin | ids / operation arrays | restore, irreversible purge, per-item errors in batch |

## Security boundary

The controlled root is set by `KORE_FILE_ROOT`; path traversal, NUL bytes, symlink escapes and the reserved `.trash` area are rejected. The trash location can be changed via `KORE_FILE_TRASH`. Error messages carry errno only and never leak absolute host paths. Four fault-injection classes cover kill -9 / timeout / disk full / cgroup limits.

## Build and verification

```bash
make check   # gofmt + go vet + go test -timeout 200s (including race)
make build && make dist
```

## Security and contributing

Do not open public issues for vulnerabilities; disclose privately per [SECURITY.md](SECURITY.md). See [CONTRIBUTING.md](CONTRIBUTING.md). MIT licensed.
