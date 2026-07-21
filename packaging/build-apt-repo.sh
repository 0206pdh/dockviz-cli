#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <deb-dir> <output-dir> <base-url>" >&2
  exit 1
fi

deb_dir=$1
output_dir=$2
base_url=$3

if [ ! -d "$deb_dir" ]; then
  echo "deb directory not found: $deb_dir" >&2
  exit 1
fi

repo_root="$output_dir/apt"
pool_dir="$repo_root/pool/main/d/dockviz"
dist_dir="$repo_root/dists/stable"
mkdir -p "$pool_dir" "$dist_dir/main/binary-amd64" "$dist_dir/main/binary-arm64"

find "$deb_dir" -maxdepth 1 -type f -name '*.deb' -exec cp {} "$pool_dir/" \;

for arch in amd64 arm64; do
  packages_file="$dist_dir/main/binary-$arch/Packages"
  (
    cd "$repo_root"
    dpkg-scanpackages --arch "$arch" pool > "dists/stable/main/binary-$arch/Packages"
  )
  gzip -kf "$packages_file"
done

cat > "$dist_dir/Release" <<EOF
Origin: dockviz
Label: dockviz
Suite: stable
Codename: stable
Architectures: amd64 arm64
Components: main
Description: dockviz APT repository
Date: $(date -Ru)
EOF

append_hash_block() {
  local algo=$1
  local cmd=$2

  echo "${algo}:" >> "$dist_dir/Release"
  while IFS= read -r -d '' file; do
    local rel size hash
    rel=${file#"$dist_dir"/}
    size=$(stat -c %s "$file")
    hash=$($cmd "$file" | awk '{print $1}')
    printf " %s %16s %s\n" "$hash" "$size" "$rel" >> "$dist_dir/Release"
  done < <(find "$dist_dir" -type f \( -name 'Packages' -o -name 'Packages.gz' \) -print0 | sort -z)
}

append_hash_block "MD5Sum" md5sum
append_hash_block "SHA256" sha256sum

if [ -n "${APT_GPG_PRIVATE_KEY:-}" ]; then
  export GNUPGHOME
  GNUPGHOME=$(mktemp -d)
  trap 'rm -rf "$GNUPGHOME"' EXIT

  printf '%s' "$APT_GPG_PRIVATE_KEY" | gpg --batch --import

  key_id=$(gpg --batch --list-secret-keys --with-colons | awk -F: '/^sec:/ {print $5; exit}')
  if [ -z "$key_id" ]; then
    echo "failed to import APT signing key" >&2
    exit 1
  fi

  gpg_args=(--batch --yes --pinentry-mode loopback)
  if [ -n "${APT_GPG_PASSPHRASE:-}" ]; then
    gpg_args+=(--passphrase "$APT_GPG_PASSPHRASE")
  fi

  gpg "${gpg_args[@]}" --armor --export "$key_id" > "$repo_root/dockviz-archive-keyring.asc"
  gpg "${gpg_args[@]}" --clearsign --digest-algo SHA256 --local-user "$key_id" \
    --output "$dist_dir/InRelease" "$dist_dir/Release"
  gpg "${gpg_args[@]}" --armor --detach-sign --local-user "$key_id" \
    --output "$dist_dir/Release.gpg" "$dist_dir/Release"
fi

cat > "$repo_root/index.html" <<EOF
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>dockviz APT Repository</title>
</head>
<body>
  <h1>dockviz APT Repository</h1>
  <p>Base URL: <code>${base_url}</code></p>
  <p>APT entry: <code>deb [signed-by=/usr/share/keyrings/dockviz-archive-keyring.gpg] ${base_url} stable main</code></p>
</body>
</html>
EOF
