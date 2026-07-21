#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <binary-path> <version> <arch> <output-deb>" >&2
  exit 1
fi

binary_path=$1
version=$2
arch=$3
output_deb=$4
package_version=${version#v}

case "$arch" in
  amd64|arm64) ;;
  *)
    echo "unsupported Debian architecture: $arch" >&2
    exit 1
    ;;
esac

if [ ! -f "$binary_path" ]; then
  echo "binary not found: $binary_path" >&2
  exit 1
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

pkg_root="$work_dir/dockviz"
mkdir -p "$pkg_root/DEBIAN" "$pkg_root/usr/bin" "$pkg_root/usr/share/doc/dockviz"

install -m 0755 "$binary_path" "$pkg_root/usr/bin/dockviz"
install -m 0644 LICENSE "$pkg_root/usr/share/doc/dockviz/LICENSE"

cat > "$pkg_root/DEBIAN/control" <<EOF
Package: dockviz
Version: $package_version
Section: utils
Priority: optional
Architecture: $arch
Maintainer: 0206pdh
Description: Real-time Docker environment dashboard for your terminal
EOF

mkdir -p "$(dirname "$output_deb")"
dpkg-deb --build --root-owner-group "$pkg_root" "$output_deb"
