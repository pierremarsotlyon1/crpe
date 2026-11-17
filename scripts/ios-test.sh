#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
destination=${IOS_DESTINATION:-}
if [ -z "$destination" ]; then
  device=$(xcrun simctl list devices available -j | python3 -c 'import json,sys; d=json.load(sys.stdin)["devices"]; phones=[p["udid"] for runtime in sorted(d,reverse=True) for p in d[runtime] if "iPhone" in p["name"]]; sys.exit("Install an iPhone simulator runtime in Xcode") if not phones else print(phones[0])')
  destination="platform=iOS Simulator,id=$device"
fi
xcodebuild -project mobile/ios/PrepaCRPE.xcodeproj -scheme PrepaCRPE \
  -configuration Debug -destination "$destination" \
  -derivedDataPath "${IOS_DERIVED_DATA:-/tmp/crpe-ios-tests}" \
  CODE_SIGN_IDENTITY=- test
