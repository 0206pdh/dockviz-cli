# dockviz-cli Troubleshooting Log

A record of bugs encountered during development and how they were resolved.

---

## 1. Network topology shows no containers

**Symptom**
The topology graph in the Networks tab shows `(no containers)` even when containers are attached to the network.

**Root cause**
Docker's `GET /networks` API (`NetworkList`) always returns an empty `Containers` field. This is not clearly documented in the Docker SDK but is the actual API behaviour.

**Fix**
Fetch the network list first, then call `NetworkInspect` per network to populate container data.

```go
// Before — Containers always empty
networks, _ := cli.NetworkList(ctx, network.ListOptions{})

// After — Inspect each network individually
detail, _ := cli.NetworkInspect(ctx, n.ID, network.InspectOptions{Verbose: false})
// detail.Containers is now populated
```

**File**: `internal/docker/networks.go`

---

## 2. Container order within a network changes on every refresh

**Symptom**
`app-network : ● api-server ─── ● nginx-proxy` becomes
`app-network : ● nginx-proxy ─── ● api-server` after a refresh.

**Root cause**
`NetworkInspect` returns `Containers` as `map[string]NetworkContainer`. Go map iteration order is **non-deterministic** — it differs on every run.

**Fix**
Convert the map to a slice and sort by container name.

```go
sort.Slice(endpoints, func(i, j int) bool {
    return endpoints[i].Name < endpoints[j].Name
})
```

**File**: `internal/docker/networks.go`

---

## 3. System networks (bridge / host / none) appear in random positions

**Symptom**
`bridge`, `host`, and `none` mixed arbitrarily among user-defined networks on every refresh.

**Root cause**
`NetworkList` does not guarantee response order. Docker manages networks internally using maps.

**Fix**
`sort.SliceStable` with a custom comparator: user-defined networks first (alphabetical), system networks pinned at the bottom in fixed order (bridge → host → none).

```go
sysOrder := map[string]int{"bridge": 0, "host": 1, "none": 2}
sort.SliceStable(result, func(i, j int) bool {
    ri, iSys := sysOrder[result[i].Name]
    rj, jSys := sysOrder[result[j].Name]
    if iSys != jSys { return !iSys }
    if iSys { return ri < rj }
    return result[i].Name < result[j].Name
})
```

**File**: `internal/docker/networks.go`

---

## 4. Image list order unstable; multiple tags collapsed into one row

**Symptom**
Tags displayed as `nginx:latest, nginx:alpine` on a single row. Row order changed on every refresh.

**Root cause**
`ImageList` response order is not guaranteed. Tags were joined with `strings.Join` into a single `Tags string` field.

**Fix**
One row per tag, sorted alphabetically by tag name.

```go
// Before
type ImageInfo struct {
    Tags string // "nginx:latest, nginx:alpine"
}

// After
type ImageInfo struct {
    Tag     string   // "nginx:latest" (this row's tag)
    AllTags []string // all tags on this image ID (for delete warning)
}
```

**Files**: `internal/docker/images.go`, `internal/tui/view.go`

---

## 5. Deleting one image tag removes the entire image

**Symptom**
Pressing `d` on `nginx:alpine` also removed `nginx:latest`.

**Root cause**
`ImageRemove` was called with `Force: true`, which removes the image regardless of other tags.

**Fix**
Changed to `Force: false`. With this setting, removing a tagged reference only untags it; the underlying image stays if other tags remain. Added a multi-tag warning to the confirmation dialog.

```go
// Before
client.ImageRemove(ctx, id, image.RemoveOptions{Force: true})

// After
client.ImageRemove(ctx, id, image.RemoveOptions{Force: false})
```

**File**: `internal/docker/images.go`

---

## 6. File edits fail on Windows with "old_string not found"

**Symptom**
Editing source files on Windows produced "old_string not found" even when the string was visibly present.

**Root cause**
`git clone` on Windows with `core.autocrlf=true` converts line endings to CRLF (`\r\n`). The edit tool matches bytes exactly; the search string used LF (`\n`) while the file contained CRLF.

**Fix**
Used a Go script to read the raw bytes, replace `\r\n` with `\n`, and write the file back. Permanent fix: add `* text=auto eol=lf` to `.gitattributes` or set `git config core.autocrlf false`.

```go
data, _ := os.ReadFile(path)
data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
os.WriteFile(path, data, 0644)
```

---

## 7. curl download yields a 9-byte "Not found" file

**Symptom**
```bash
curl https://github.com/.../releases/latest/download/dockviz-linux-amd64 -o dockviz
# Result: 9-byte file containing "Not found"
```

**Root cause**
GitHub Releases CDN returns a **302 redirect** to the actual CDN URL. Without `-L`, curl saves the 302 response body instead of following the redirect.

**Fix**
Add `-L` (follow redirects) and `-s` (silent mode).

```bash
# Before
curl https://github.com/.../dockviz-linux-amd64 -o dockviz

# After
curl -sL https://github.com/.../dockviz-linux-amd64 -o dockviz
```

---

## 8. `dockviz --version` reports old version after update

**Symptom**
After installing a new binary to `/usr/local/bin/dockviz`, `--version` still reported the previous version.

**Root cause**
An older binary existed at a different PATH location (e.g. `/usr/bin/dockviz`) that the shell resolved first.

**Fix**
```bash
rm $(which dockviz)            # remove the stale binary
mv dockviz /usr/local/bin/dockviz  # install new binary to canonical location
```

Standardised the install command to always target `/usr/local/bin` to avoid this.

---

## 9. GitHub Actions overwrites manually written release notes

**Symptom**
After writing release notes with `gh release edit`, the next tag push caused CI to replace them with auto-generated content.

**Root cause**
`softprops/action-gh-release` in `.github/workflows/release.yml` had `generate_release_notes: true`, which overwrites the release body even when the release already exists.

**Fix**
```yaml
# Before
generate_release_notes: true

# After
generate_release_notes: false
```

Release notes are now written manually after CI completes using `gh release edit vX.Y.Z --notes "..."`.

**File**: `.github/workflows/release.yml`

---

## 10. `docker run` hangs at `>` prompt

**Symptom**
```bash
docker run -d --name api-server \
  --network app-network \
  -p 3000:3000 node:alpine sh -c "..."
>
```
The command never completes; the shell keeps waiting for more input.

**Root cause**
The shell interpreted the backslash line continuation as an incomplete command — either an unclosed quote or trailing whitespace after a `\`.

**Fix**
Press `Ctrl+C` to cancel, then run the command as a single line.

```bash
docker run -d --name api-server --network app-network -p 3000:3000 node:alpine sh -c "while true; do sleep 1; done"
```
