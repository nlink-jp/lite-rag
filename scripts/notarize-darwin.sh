#!/bin/sh
# notarize-darwin.sh — submit a zip-packaged darwin binary to Apple's
# notary service and wait for the verdict.
#
# Usage:
#   notarize-darwin.sh <zip-path> [profile-name]
#
# Profile name defaults to the value of NOTARY_PROFILE env, then to
# `nlink-jp-notary`. Credentials are stored once per machine via:
#
#   xcrun notarytool store-credentials nlink-jp-notary \
#       --key <p8>  --key-id <id>  --issuer <uuid>
#
# Behaviour:
#   - Skips on non-Darwin hosts (cross-compile from Linux/etc.)
#   - Skips when the keychain profile isn't present (other
#     contributors / CI without credentials still produce the zip,
#     just un-notarised). A one-line warning is printed.
#   - On failure, prints the Apple-returned log and exits non-zero
#     so a release pipeline can stop.
#
# Why we don't staple: notarisation of bare CLI binaries inside a
# zip cannot be stapled — `stapler staple` only works on app
# bundles, dmgs, and pkgs. The notarisation ticket lives on Apple's
# servers and macOS checks it online the first time the binary is
# launched on a given machine. This is the standard pattern for
# non-bundle distributables (cf. the official notarytool docs).

set -e

ZIP="${1:?Usage: $0 <zip-path> [profile]}"
PROFILE="${2:-${NOTARY_PROFILE:-nlink-jp-notary}}"

if [ "$(uname)" != "Darwin" ]; then
  exit 0
fi

if [ ! -f "$ZIP" ]; then
  echo "[notarize] $ZIP not found, skipping" >&2
  exit 0
fi

# Probe the keychain profile cheaply (notarytool has no dedicated
# "is profile present" command). `history` returns quickly without
# uploading anything, so we use it as a liveness check.
if ! xcrun notarytool history --keychain-profile "$PROFILE" >/dev/null 2>&1; then
  echo "[notarize] Keychain profile '$PROFILE' not found; $ZIP will ship un-notarised" >&2
  echo "[notarize] To enable, run once per machine:" >&2
  echo "[notarize]   xcrun notarytool store-credentials $PROFILE --key <p8> --key-id <id> --issuer <uuid>" >&2
  exit 0
fi

echo "[notarize] Submitting $ZIP to Apple notary service (this typically takes 30s-2m)..."
SUBMISSION_OUT=$(xcrun notarytool submit "$ZIP" --keychain-profile "$PROFILE" --wait 2>&1) || {
  echo "[notarize] $ZIP: submission failed" >&2
  echo "$SUBMISSION_OUT" >&2
  exit 1
}

echo "$SUBMISSION_OUT"

# notarytool exits 0 on Accepted, non-zero otherwise. As an extra
# guard, parse the status line in the output and fail explicitly
# on anything other than "Accepted" so a release pipeline halts
# even if Apple shifts exit-code semantics in a future release.
if printf '%s\n' "$SUBMISSION_OUT" | grep -q 'status: Accepted'; then
  echo "[notarize] $ZIP: Accepted"
  exit 0
fi

echo "[notarize] $ZIP: notarisation did not succeed (see status above)" >&2
exit 1
