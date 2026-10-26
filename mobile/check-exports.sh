#!/usr/bin/env bash
set -euo pipefail

# Guards the gomobile binding surface. Every exported symbol under mobile/
# becomes a method the Android AAR and the iOS xcframework publish, so
# removing or renaming one breaks the apps at link time (iOS) or at run
# time (Android). The snapshot makes that a deliberate, reviewed act.
#
#   bash mobile/check-exports.sh           compare against the snapshot
#   bash mobile/check-exports.sh --update  accept the current surface

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
snapshot="${repo_root}/mobile/exports.txt"

# Only packages that git tracks are part of a release. A directory the
# repository ignores exists on a developer's disk but not in a fresh
# checkout, so including it would make the snapshot unreproducible.
tracked_packages() {
    local pkg short
    for pkg in $(cd "${repo_root}" && go list ./mobile/...); do
        short="${pkg#github.com/kernelflowlabs/wallet-lib/}"
        if (cd "${repo_root}" && git ls-files -- "${short}") | grep -q '\.go$'; then
            echo "${pkg}"
        fi
    done
}

collect() {
    local pkg short
    for pkg in $(tracked_packages); do
        short="${pkg#github.com/kernelflowlabs/wallet-lib/}"
        (cd "${repo_root}" && go doc -short "${pkg}" 2>/dev/null) |
            sed -E "s|^|${short}\t|"
    done | LC_ALL=C sort
}

if [[ "${1:-}" == "--update" ]]; then
    collect > "${snapshot}"
    echo "Snapshot updated: $(wc -l < "${snapshot}") exported symbols"
    exit 0
fi

if [[ ! -f "${snapshot}" ]]; then
    echo "Missing ${snapshot}; run: bash mobile/check-exports.sh --update" >&2
    exit 1
fi

current="$(mktemp)"
trap 'rm -f "${current}"' EXIT
collect > "${current}"

if diff -u "${snapshot}" "${current}" > /dev/null; then
    echo "Mobile binding surface unchanged: $(wc -l < "${snapshot}") exported symbols"
    exit 0
fi

echo "The gomobile binding surface changed." >&2
echo >&2
diff -u "${snapshot}" "${current}" | sed -n '4,$p' >&2
echo >&2
cat >&2 <<'EOF'
A line starting with "-" is a symbol the published AAR and xcframework
will no longer expose. Apps calling it fail to link on iOS and raise
NoSuchMethodError on Android.

If the change is intended, record it:

    bash mobile/check-exports.sh --update

and commit mobile/exports.txt with the change.
EOF
exit 1
