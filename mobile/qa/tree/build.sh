#!/bin/sh
set -eu
src=$(cd "$(dirname "$0")" && pwd)
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
build="$ANDROID_HOME/build-tools/37.0.0"
platform="$ANDROID_HOME/platforms/android-37.0/android.jar"
mkdir "$out/classes" "$out/dex"
"$JAVA_HOME/bin/javac" -source 8 -target 8 -classpath "$platform" -d "$out/classes" "$src/Tree.java"
"$JAVA_HOME/bin/jar" cf "$out/classes.jar" -C "$out/classes" .
"$build/d8" --lib "$platform" --min-api 28 --output "$out/dex" "$out/classes.jar"
"$build/aapt2" link -o "$out/test.apk" -I "$platform" --manifest "$src/AndroidManifest.xml"
(cd "$out/dex" && zip -q "$out/test.apk" classes.dex)
"$build/zipalign" 4 "$out/test.apk" "$out/aligned.apk"
# The temporary key signs only the short-lived, self-instrumenting test APK.
"$JAVA_HOME/bin/keytool" -genkeypair -alias qa -keystore "$out/test.p12" -storepass crpe-test -keypass crpe-test -keyalg RSA -validity 1 -dname CN=CRPE-Oral-QA >/dev/null 2>&1
"$build/apksigner" sign --ks "$out/test.p12" --ks-pass pass:crpe-test --out "$1" "$out/aligned.apk"
