#!/usr/bin/env bash
set -euo pipefail

version="${1:-0.1.0}"
arch="${2:-amd64}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="$root_dir/dist"
build_dir="$root_dir/build/release"
binary_dir="$build_dir/bin"
binary="$binary_dir/cpq"

export GOCACHE="${GOCACHE:-/tmp/gocache}"

rm -rf "$dist_dir" "$build_dir"
mkdir -p "$dist_dir" "$binary_dir"

cd "$root_dir"
go build -trimpath -ldflags="-s -w" -o "$binary" .

tar_dir="$build_dir/cpq-$version-linux-$arch"
mkdir -p "$tar_dir"
cp "$binary" "$tar_dir/cpq"
cp README.md LICENSE "$tar_dir/"
tar -C "$build_dir" -czf "$dist_dir/cpq-$version-linux-$arch.tar.gz" "cpq-$version-linux-$arch"

pkg_root="$build_dir/deb/cpq"
mkdir -p \
  "$pkg_root/DEBIAN" \
  "$pkg_root/usr/bin" \
  "$pkg_root/usr/share/applications" \
  "$pkg_root/usr/share/icons/hicolor/256x256/apps" \
  "$pkg_root/usr/share/doc/cpq"
cp "$binary" "$pkg_root/usr/bin/cpq"
cp packaging/linux/cpq.desktop "$pkg_root/usr/share/applications/cpq.desktop"
cp internal/tray/icon.png "$pkg_root/usr/share/icons/hicolor/256x256/apps/cpq.png"
cp README.md LICENSE "$pkg_root/usr/share/doc/cpq/"
cat > "$pkg_root/DEBIAN/control" <<CONTROL
Package: cpq
Version: $version
Section: utils
Priority: optional
Architecture: $arch
Maintainer: Michael <267470902+Sora378@users.noreply.github.com>
Depends: libc6, libgl1, libx11-6, libxcursor1, libxrandr2, libxinerama1, libxi6, libxxf86vm1, libsqlite3-0, libsecret-1-0
Description: Coding-plan quota tray utility
 CPQ is a local Linux tray utility for showing coding-plan quota usage across
 supported AI providers.
CONTROL
dpkg-deb --build --root-owner-group "$pkg_root" "$dist_dir/cpq_${version}_${arch}.deb"

if command -v rpmbuild >/dev/null 2>&1; then
  rpm_top="$build_dir/rpmbuild"
  mkdir -p "$rpm_top/BUILD" "$rpm_top/RPMS" "$rpm_top/SOURCES" "$rpm_top/SPECS" "$rpm_top/SRPMS"
  rpm_payload="$build_dir/rpm-payload/cpq-$version"
  mkdir -p \
    "$rpm_payload/usr/bin" \
    "$rpm_payload/usr/share/applications" \
    "$rpm_payload/usr/share/icons/hicolor/256x256/apps" \
    "$rpm_payload/usr/share/doc/cpq"
  cp "$binary" "$rpm_payload/usr/bin/cpq"
  cp packaging/linux/cpq.desktop "$rpm_payload/usr/share/applications/cpq.desktop"
  cp internal/tray/icon.png "$rpm_payload/usr/share/icons/hicolor/256x256/apps/cpq.png"
  cp README.md LICENSE "$rpm_payload/usr/share/doc/cpq/"
  tar -C "$build_dir/rpm-payload" -czf "$rpm_top/SOURCES/cpq-$version.tar.gz" "cpq-$version"
  cat > "$rpm_top/SPECS/cpq.spec" <<SPEC
Name: cpq
Version: $version
Release: 1%{?dist}
Summary: Coding-plan quota tray utility
License: MIT
URL: https://github.com/Sora378/codingplantracker
Source0: %{name}-%{version}.tar.gz
Requires: glibc, mesa-libGL, libX11, libXcursor, libXrandr, libXinerama, libXi, libXxf86vm, sqlite-libs, libsecret

%description
CPQ is a local Linux tray utility for showing coding-plan quota usage across supported AI providers.

%prep
%setup -q

%build

%install
mkdir -p %{buildroot}
cp -a usr %{buildroot}/

%files
%license /usr/share/doc/cpq/LICENSE
%doc /usr/share/doc/cpq/README.md
/usr/bin/cpq
/usr/share/applications/cpq.desktop
/usr/share/icons/hicolor/256x256/apps/cpq.png

%changelog
* Thu Apr 30 2026 Michael <267470902+Sora378@users.noreply.github.com> - $version-1
- Initial public release
SPEC
  rpmbuild --define "_topdir $rpm_top" -bb "$rpm_top/SPECS/cpq.spec"
  find "$rpm_top/RPMS" -type f -name '*.rpm' -exec cp {} "$dist_dir/" \;
fi

appdir="$build_dir/AppDir"
mkdir -p \
  "$appdir/usr/bin" \
  "$appdir/usr/share/applications" \
  "$appdir/usr/share/icons/hicolor/256x256/apps"
cp "$binary" "$appdir/usr/bin/cpq"
cp packaging/linux/cpq.desktop "$appdir/usr/share/applications/cpq.desktop"
cp internal/tray/icon.png "$appdir/usr/share/icons/hicolor/256x256/apps/cpq.png"
cp packaging/linux/cpq.desktop "$appdir/cpq.desktop"
cp internal/tray/icon.png "$appdir/cpq.png"
cat > "$appdir/AppRun" <<'APPRUN'
#!/usr/bin/env sh
exec "$APPDIR/usr/bin/cpq" "$@"
APPRUN
chmod +x "$appdir/AppRun"

if command -v appimagetool >/dev/null 2>&1; then
  ARCH=x86_64 appimagetool "$appdir" "$dist_dir/cpq-$version-x86_64.AppImage"
fi

(
  cd "$dist_dir"
  sha256sum * > SHA256SUMS
)
