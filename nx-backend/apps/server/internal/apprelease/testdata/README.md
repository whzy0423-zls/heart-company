# APK test fixtures

These APKs are public test fixtures only. `signed-minimal.apk` is signed with a disposable RSA key created solely for this fixture; it is not a production release key and must never be used to publish an application. The temporary JKS is deleted at the end of generation and is intentionally not committed.

Both fixtures contain this manifest metadata:

- package: `com.xinzhili.nine_xing_app`
- version name: `1.2.3`
- version code: `123`
- minimum SDK: `23`
- target SDK: `35`
- application code: disabled (`android:hasCode="false"`)

The committed fixtures have these digests:

- `signed-minimal.apk` SHA-256: `ff0bee8b9c297d4ecd1f9d248ea6408ba023bb899591a0146dbe0337d059a505`
- `unsigned-minimal.apk` SHA-256: `3d5e7290b081a98c50a8fdffeb8b216730bd4ab74e610683d4514cbb5ed8d9bc`
- signing certificate SHA-256: `9fa7d5b8a76d2cb8869cf0613c0237e02b66ea95aa775a6ff386204aab8b0162`

Regenerate the fixtures from the repository root with Android SDK platform 35 and build-tools 35.0.0 (adjust only `build_tools` if that installed version differs):

```bash
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
tmp="$(mktemp -d)"
cat > "$tmp/AndroidManifest.xml" <<'EOF'
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
  package="com.xinzhili.nine_xing_app"
  android:versionCode="123"
  android:versionName="1.2.3">
  <uses-sdk android:minSdkVersion="23" android:targetSdkVersion="35" />
  <application android:label="九型芯之力测试包" android:hasCode="false" />
</manifest>
EOF

build_tools="$ANDROID_HOME/build-tools/35.0.0"
"$build_tools/aapt2" link \
  -I "$ANDROID_HOME/platforms/android-35/android.jar" \
  --manifest "$tmp/AndroidManifest.xml" \
  -o "$tmp/unsigned.apk"
keytool -genkeypair -noprompt \
  -keystore "$tmp/fixture.jks" -storepass fixturepass -keypass fixturepass \
  -alias fixture -keyalg RSA -keysize 2048 -validity 3650 \
  -dname "CN=Nine Xing Test Fixture,O=Local Tests,C=CN"
"$build_tools/zipalign" -f 4 "$tmp/unsigned.apk" "$tmp/aligned.apk"
cp "$tmp/aligned.apk" \
  nx-backend/apps/server/internal/apprelease/testdata/unsigned-minimal.apk
"$build_tools/apksigner" sign \
  --ks "$tmp/fixture.jks" --ks-pass pass:fixturepass --key-pass pass:fixturepass \
  --v4-signing-enabled false \
  --out nx-backend/apps/server/internal/apprelease/testdata/signed-minimal.apk \
  "$tmp/aligned.apk"
"$build_tools/apksigner" verify --verbose --print-certs \
  nx-backend/apps/server/internal/apprelease/testdata/signed-minimal.apk
if "$build_tools/apksigner" verify \
  nx-backend/apps/server/internal/apprelease/testdata/unsigned-minimal.apk; then
  echo "unsigned fixture unexpectedly verified" >&2
  rm -rf "$tmp"
  exit 1
fi
rm -rf "$tmp"
```

The generated key and certificate are random, so regenerating the signed fixture changes both its file digest and certificate digest. Update the values above and the exact certificate assertion in `apk_test.go` when intentionally replacing the fixture.
