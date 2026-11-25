# Third-party notices

The Podracer Bridge source is MIT licensed. It does not redistribute LINSLABS paid files or incorporate code from JaffaQuake's GPL-licensed bridge.

The compiled application includes the Go runtime and standard library, covered by the Go BSD-style license: https://go.dev/LICENSE. A copy is included in the source distribution as `licenses/GO-LICENSE`.

The AppImage includes the official AppImage type2 runtime, distributed under its upstream license. Runtime sources and bundled-library licensing are maintained at https://github.com/AppImage/type2-runtime. Its license file is included as `licenses/APPIMAGE-RUNTIME-LICENSE`. The exact upstream commit and downloaded runtime checksum are recorded in `BUILD_PROVENANCE.json`; corresponding runtime source is included with the source bundle under `third_party` when available.

The Windows SquashFS tool is a build-only tool, not bundled into the AppImage. Source and license: https://github.com/michaelkschmidt/squashfs-tools. The Go compiler is also build-only; the compiler itself is not bundled. No other third-party application libraries are used.

LINSLABS, Arduino, Steam, Xbox, Bazzite, and Star Wars names identify hardware, software, or intended compatibility. This independent application is not affiliated with or endorsed by those projects or brands.
